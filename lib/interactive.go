package lib

import (
	"encoding/json"
	"errors"
	"fmt"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

type MessageContent struct {
	Text               string             `json:"text,omitempty"`
	Footer             string             `json:"footer,omitempty"`
	Title              string             `json:"title,omitempty"`
	Subtitle           string             `json:"subtitle,omitempty"`
	InteractiveButtons []*ButtonConfig    `json:"interactiveButtons,omitempty"`
	ContextInfo        *waE2E.ContextInfo `json:"contextInfo,omitempty"`
}

type ButtonConfig struct {
	Name             string      `json:"name"`
	ButtonParamsJson string      `json:"buttonParamsJson"`
	ButtonId         string      `json:"buttonId,omitempty"`
	ButtonText       *ButtonText `json:"buttonText,omitempty"`
}

type ButtonText struct {
	DisplayText string `json:"displayText,omitempty"`
}

func BuildInteractiveFromContent(content *MessageContent) *waE2E.InteractiveMessage {
	var buttons []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton

	for _, b := range content.InteractiveButtons {
		name := b.Name

		if name == "" {
			if b.ButtonId != "" && b.ButtonText != nil {
				display := b.ButtonText.DisplayText
				if display == "" {
					display = "Tombol"
				}
				jsonParams, _ := json.Marshal(map[string]string{
					"display_text": display,
					"id":           b.ButtonId,
				})
				name = "quick_reply"
				b.ButtonParamsJson = string(jsonParams)
			} else {
				name = "quick_reply"
			}
		}

		buttons = append(buttons, &waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
			Name:             proto.String(name),
			ButtonParamsJSON: proto.String(b.ButtonParamsJson),
		})
	}

	nativeFlow := &waE2E.InteractiveMessage_NativeFlowMessage{
		Buttons:        buttons,
		MessageVersion: proto.Int32(0),
	}

	im := &waE2E.InteractiveMessage{
		InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{
			NativeFlowMessage: nativeFlow,
		},
		Body: &waE2E.InteractiveMessage_Body{
			Text: proto.String(content.Text),
		},
	}

	if content.Title != "" || content.Subtitle != "" {
		im.Header = &waE2E.InteractiveMessage_Header{
			Title:    proto.String(content.Title),
			Subtitle: proto.String(content.Subtitle),
		}
	}

	if content.Footer != "" {
		im.Footer = &waE2E.InteractiveMessage_Footer{
			Text: proto.String(content.Footer),
		}
	}

	if content.ContextInfo != nil {
		im.ContextInfo = content.ContextInfo
	}

	return im
}

func MakeNativeFlowButton(name string, params map[string]interface{}) (*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton, error) {
	jsonBytes, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("gagal marshal buttonParams untuk %s: %w", name, err)
	}
	return &waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
		Name:             proto.String(name),
		ButtonParamsJSON: proto.String(string(jsonBytes)),
	}, nil
}

var AllowedNames = map[string]bool{
	"quick_reply": true, "cta_url": true, "cta_copy": true, "cta_call": true,
	"cta_catalog": true, "cta_reminder": true, "cta_cancel_reminder": true,
	"address_message": true, "send_location": true, "open_webview": true,
	"mpm": true, "wa_payment_transaction_details": true,
	"automated_greeting_message_view_catalog": true, "galaxy_message": true,
	"single_select": true,
}

var RequiredFieldsMap = map[string][]string{
	"cta_url":                        {"display_text", "url"},
	"cta_copy":                       {"display_text", "copy_code"},
	"cta_call":                       {"display_text", "phone_number"},
	"cta_catalog":                    {"business_phone_number"},
	"cta_reminder":                   {"display_text"},
	"cta_cancel_reminder":            {"display_text"},
	"address_message":                {"display_text"},
	"send_location":                  {"display_text"},
	"open_webview":                   {"title", "link"},
	"mpm":                            {"product_id"},
	"wa_payment_transaction_details": {"transaction_id"},
	"automated_greeting_message_view_catalog": {"business_phone_number", "catalog_product_id"},
	"galaxy_message": {"flow_token", "flow_id"},
	"single_select":  {"title", "sections"},
	"quick_reply":    {"display_text", "id"},
}

func ValidateButton(btn *waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton, idx int) (errs []error, warns []string) {
	if btn == nil {
		errs = append(errs, fmt.Errorf("buttons[%d] adalah null", idx))
		return
	}

	name := btn.GetName()
	if name == "" {
		errs = append(errs, fmt.Errorf("buttons[%d] name tidak boleh kosong", idx))
		name = "quick_reply"
	}

	if !AllowedNames[name] {
		warns = append(warns, fmt.Sprintf("buttons[%d] name '%s' tidak umum", idx, name))
	}

	paramsJSON := btn.GetButtonParamsJSON()
	if paramsJSON == "" {
		errs = append(errs, fmt.Errorf("buttons[%d] buttonParamsJSON kosong", idx))
		return
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(paramsJSON), &parsed); err != nil {
		errs = append(errs, fmt.Errorf("buttons[%d] buttonParamsJSON bukan JSON valid: %v", idx, err))
		return
	}

	if req := RequiredFieldsMap[name]; len(req) > 0 {
		for _, f := range req {
			if _, ok := parsed[f]; !ok {
				errs = append(errs, fmt.Errorf("buttons[%d] (%s) field wajib '%s' tidak ditemukan", idx, name, f))
			}
		}
	}

	return
}

func ValidateInteractiveMessage(im *waE2E.InteractiveMessage) (errs []error, warns []string) {
	if im == nil {
		errs = append(errs, errors.New("interactiveMessage tidak boleh null"))
		return
	}

	nf, ok := im.InteractiveMessage.(*waE2E.InteractiveMessage_NativeFlowMessage_)
	if !ok || nf.NativeFlowMessage == nil {
		errs = append(errs, errors.New("interactiveMessage.nativeFlowMessage tidak ditemukan"))
		return
	}

	if len(nf.NativeFlowMessage.Buttons) == 0 {
		errs = append(errs, errors.New("nativeFlowMessage.buttons harus ada (minimal 1 tombol)"))
		return
	}

	for i, btn := range nf.NativeFlowMessage.Buttons {
		e, w := ValidateButton(btn, i)
		errs = append(errs, e...)
		warns = append(warns, w...)
	}

	if im.GetBody().GetText() == "" {
		warns = append(warns, "interactiveMessage.body.text kosong — pesan mungkin kurang jelas")
	}

	return
}
