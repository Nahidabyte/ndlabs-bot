package plugins

import (
	"bytes"
	"encoding/json"
	"strings"

	"wa-bot/lib"
	"wa-bot/plugin"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func init() {
	plugin.RegisterPlugin(plugin.Cmd{
		Name:     "raw",
		Category: "tools",
		Alias:    []string{"getraw", "msginfo"},
		Desc:     "Mendapatkan raw JSON Protobuf dari sebuah pesan dalam bentuk File",
		Exec:     rawCmd,
	})

	plugin.RegisterPlugin(plugin.Cmd{
		Name:     "sendraw",
		Category: "tools",
		Alias:    []string{"executeraw", "pushraw"},
		Desc:     "Mengirim pesan berdasarkan payload raw JSON (Auto Reply & Auto Bypass Interactive)",
		Exec:     sendRawCmd,
	})
}

func rawCmd(ctx *plugin.CommandContext) {
	var msgToExtract *waProto.Message

	if ctx.Msg.Quoted != nil {

		if ctx.Msg.Socket.GetCachedMessage != nil && ctx.Msg.Quoted.StanzaID != "" {
			if cached := ctx.Msg.Socket.GetCachedMessage(ctx.Msg.Quoted.StanzaID); cached != nil {
				msgToExtract = cached.Message
			}
		}

		if msgToExtract == nil {
			msgToExtract = ctx.Msg.RawMessage.Message.GetExtendedTextMessage().GetContextInfo().GetQuotedMessage()
		}
	} else {
		msgToExtract = ctx.Msg.RawMessage.Message
	}

	if msgToExtract == nil {
		ctx.Msg.Reply("❌ Tidak dapat menemukan struktur pesan untuk diekstrak.")
		return
	}

	marshaller := protojson.MarshalOptions{
		Multiline:       true,
		EmitUnpopulated: false,
	}

	jsonBytes, err := marshaller.Marshal(msgToExtract)
	if err != nil {
		ctx.Msg.Reply("❌ Gagal mengekstrak pesan ke JSON:\n" + err.Error())
		return
	}

	if data, filename := extractRichSections(msgToExtract); len(data) > 0 {
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, data, "", "  "); err != nil {
			pretty.Write(data)
		}
		filename = strings.TrimSuffix(filename, ".json") + ".txt"
		ctx.Msg.SendDocumentBytes(pretty.Bytes(), filename, "text/plain",
			"📐 Layout UI terdeteksi. Kirim file ini buat direplikasi jadi builder baru.")
	}

	snippet := "nopal.RelayMessage(m.JID, `\n" + string(jsonBytes) + "\n`)\n"
	if _, err := ctx.Msg.SendDocumentBytes([]byte(snippet), "raw_message.txt", "text/plain",
		"🧬 Snippet relay siap pakai. Paste ke `> ` (eval) atau reply file ini pakai .sendraw buat replay."); err != nil {

		ctx.Msg.Reply("```go\n" + snippet + "```")
	}
}

func unwrapMessage(msg *waProto.Message) *waProto.Message {
	if msg == nil {
		return nil
	}
	if ephemeral := msg.GetEphemeralMessage(); ephemeral != nil {
		return unwrapMessage(ephemeral.GetMessage())
	}
	if viewOnce := msg.GetViewOnceMessage(); viewOnce != nil {
		return unwrapMessage(viewOnce.GetMessage())
	}
	if viewOnceV2 := msg.GetViewOnceMessageV2(); viewOnceV2 != nil {
		return unwrapMessage(viewOnceV2.GetMessage())
	}
	if docWithCaption := msg.GetDocumentWithCaptionMessage(); docWithCaption != nil {
		return unwrapMessage(docWithCaption.GetMessage())
	}
	if botForwarded := msg.GetBotForwardedMessage(); botForwarded != nil {

		if inner := botForwarded.GetMessage(); inner != nil {
			return unwrapMessage(inner)
		}
	}
	return msg
}

func extractRichSections(msg *waProto.Message) ([]byte, string) {
	realMsg := unwrapMessage(msg)

	if rr := realMsg.GetRichResponseMessage(); rr != nil {
		if d := rr.GetUnifiedResponse().GetData(); len(d) > 0 {
			return d, "rich_sections.json"
		}
	}

	if im := realMsg.GetInteractiveMessage(); im != nil {
		if nfm := im.GetNativeFlowMessage(); nfm != nil {

			data, err := protojson.MarshalOptions{Multiline: true, EmitUnpopulated: true}.Marshal(nfm)
			if err == nil && len(data) > 0 {
				return data, "interactive_layout.json"
			}
		}
	}

	if list := realMsg.GetListMessage(); list != nil {
		data, err := protojson.MarshalOptions{Multiline: true, EmitUnpopulated: true}.Marshal(list)
		if err == nil {
			return data, "list_layout.json"
		}
	}
	if template := realMsg.GetTemplateMessage(); template != nil {
		data, err := protojson.MarshalOptions{Multiline: true, EmitUnpopulated: true}.Marshal(template)
		if err == nil {
			return data, "template_layout.json"
		}
	}

	return nil, ""
}

func extractDocumentMessage(msg *waProto.Message) *waProto.DocumentMessage {
	if msg == nil {
		return nil
	}
	if doc := msg.GetDocumentMessage(); doc != nil {
		return doc
	}
	if docWithCaption := msg.GetDocumentWithCaptionMessage(); docWithCaption != nil {
		return extractDocumentMessage(docWithCaption.GetMessage())
	}
	if viewOnce := msg.GetViewOnceMessage(); viewOnce != nil {
		return extractDocumentMessage(viewOnce.GetMessage())
	}
	if viewOnceV2 := msg.GetViewOnceMessageV2(); viewOnceV2 != nil {
		return extractDocumentMessage(viewOnceV2.GetMessage())
	}
	if ephemeral := msg.GetEphemeralMessage(); ephemeral != nil {
		return extractDocumentMessage(ephemeral.GetMessage())
	}
	return nil
}

func unwrapRelaySnippet(s string) string {

	return lib.CleanRelayJSON(s)
}

func isButtonMessage(msg *waProto.Message) bool {
	if msg == nil {
		return false
	}
	return msg.GetInteractiveMessage() != nil ||
		msg.GetButtonsMessage() != nil ||
		msg.GetTemplateMessage() != nil ||
		msg.GetListMessage() != nil
}

func isRichMessage(msg *waProto.Message) bool {
	if msg == nil {
		return false
	}
	if msg.GetRichResponseMessage() != nil {
		return true
	}
	if bf := msg.GetBotForwardedMessage(); bf != nil {
		if bf.GetMessage().GetRichResponseMessage() != nil {
			return true
		}
	}
	return false
}

func isMetaAIJID(jid types.JID) bool {
	return jid.User == "0" || jid.Server == "bot" || jid.User == "13135550002"
}

func sendRawCmd(ctx *plugin.CommandContext) {
	var jsonStr string
	var replyContext *waProto.ContextInfo

	rawArgs := strings.TrimSpace(ctx.RawArgs)

	if ctx.Msg.Quoted != nil {
		quotedMsgInfo := ctx.Msg.RawMessage.Message.GetExtendedTextMessage().GetContextInfo()
		quotedPayload := quotedMsgInfo.GetQuotedMessage()

		if rawArgs == "" {

			if docMsg := extractDocumentMessage(quotedPayload); docMsg != nil {
				ctx.Msg.React("⏳")
				fileBytes, err := ctx.Msg.Socket.Client.Download(ctx.Ctx, docMsg)
				if err != nil {
					ctx.Msg.Reply("❌ Gagal mengunduh file:\n" + err.Error())
					return
				}
				jsonStr = string(fileBytes)
			} else {
				jsonStr = ctx.Msg.Quoted.Text
			}
		} else {

			jsonStr = rawArgs
			replyContext = &waProto.ContextInfo{
				StanzaID:      proto.String(ctx.Msg.Quoted.StanzaID),
				Participant:   proto.String(ctx.Msg.Quoted.Sender.String()),
				QuotedMessage: quotedPayload,
			}
		}
	} else {

		jsonStr = rawArgs
	}

	jsonStr = strings.TrimSpace(jsonStr)

	if jsonStr == "" {
		ctx.Msg.Reply("❌ Format salah!\n\n*Cara penggunaan:*\n1. Balas file .json dengan *.sendraw*\n2. Balas pesan teman dengan *.sendraw {\"conversation\": \"Halo\"}*")
		return
	}

	jsonStr = unwrapRelaySnippet(jsonStr)

	var newMsg waProto.Message

	unmarshaller := protojson.UnmarshalOptions{DiscardUnknown: true}
	err := unmarshaller.Unmarshal([]byte(jsonStr), &newMsg)
	if err != nil {
		ctx.Msg.Reply("❌ JSON tidak valid:\n" + err.Error())
		return
	}

	if replyContext != nil {
		if newMsg.ExtendedTextMessage != nil {
			newMsg.ExtendedTextMessage.ContextInfo = replyContext
		} else if newMsg.ImageMessage != nil {
			newMsg.ImageMessage.ContextInfo = replyContext
		} else if newMsg.VideoMessage != nil {
			newMsg.VideoMessage.ContextInfo = replyContext
		} else if newMsg.DocumentMessage != nil {
			newMsg.DocumentMessage.ContextInfo = replyContext
		} else if newMsg.InteractiveMessage != nil {
			newMsg.InteractiveMessage.ContextInfo = replyContext
		} else if newMsg.ButtonsMessage != nil {
			newMsg.ButtonsMessage.ContextInfo = replyContext
		} else if newMsg.TemplateMessage != nil {
			newMsg.TemplateMessage.ContextInfo = replyContext
		} else if newMsg.ListMessage != nil {
			newMsg.ListMessage.ContextInfo = replyContext
		} else if newMsg.Conversation != nil {

			newMsg.ExtendedTextMessage = &waProto.ExtendedTextMessage{
				Text:        newMsg.Conversation,
				ContextInfo: replyContext,
			}
			newMsg.Conversation = nil
		}
	}

	isRich := isRichMessage(&newMsg)
	if !isRich && ctx.Msg.Quoted != nil && isMetaAIJID(ctx.Msg.Quoted.Sender) {
		isRich = true
	}

	var extra []whatsmeow.SendRequestExtra
	switch {
	case isRich:

	case isButtonMessage(&newMsg):
		bypassNodes := lib.GetLumiBypassNodes()
		extra = append(extra, whatsmeow.SendRequestExtra{AdditionalNodes: &bypassNodes})
	}

	_, err = ctx.Msg.Socket.Client.SendMessage(ctx.Ctx, ctx.Msg.JID, &newMsg, extra...)
	if err != nil {
		ctx.Msg.Reply("❌ Gagal mengirim pesan RAW:\n" + err.Error())
	}
}
