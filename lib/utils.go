package lib

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
)

var CachedName sync.Map

var (
	AntiTagLimits   = make(map[string]int)
	AntiTagLimitsMu sync.RWMutex

	MassTagUsage   = make(map[string]int)
	MassTagUsageMu sync.RWMutex
)

func init() {
	b, err := os.ReadFile("antitag_limits.json")
	if err == nil {
		json.Unmarshal(b, &AntiTagLimits)
	}
	b2, err2 := os.ReadFile("masstag_usage.json")
	if err2 == nil {
		json.Unmarshal(b2, &MassTagUsage)
	}
}

func GetAntiTagLimit(jid string) int {
	AntiTagLimitsMu.RLock()
	defer AntiTagLimitsMu.RUnlock()
	return AntiTagLimits[jid]
}

func SetAntiTagLimit(jid string, limit int) {
	AntiTagLimitsMu.Lock()
	defer AntiTagLimitsMu.Unlock()
	if limit <= 0 {
		delete(AntiTagLimits, jid)
	} else {
		AntiTagLimits[jid] = limit
	}
	b, _ := json.Marshal(AntiTagLimits)
	os.WriteFile("antitag_limits.json", b, 0644)
}

func IncrementMassTagUsage(groupID, userID string) int {
	MassTagUsageMu.Lock()
	defer MassTagUsageMu.Unlock()
	today := time.Now().Format("2006-01-02")

	for k := range MassTagUsage {
		if !strings.HasSuffix(k, today) {
			delete(MassTagUsage, k)
		}
	}

	key := groupID + "|" + userID + "|" + today
	MassTagUsage[key]++
	count := MassTagUsage[key]

	b, _ := json.Marshal(MassTagUsage)
	os.WriteFile("masstag_usage.json", b, 0644)
	return count
}

func ResetMassTagUsage(groupID, userID string) {
	MassTagUsageMu.Lock()
	defer MassTagUsageMu.Unlock()
	today := time.Now().Format("2006-01-02")
	key := groupID + "|" + userID + "|" + today
	delete(MassTagUsage, key)
	b, _ := json.Marshal(MassTagUsage)
	os.WriteFile("masstag_usage.json", b, 0644)
}

func GetText(msg *waE2E.Message) (text string, ok bool) {
	if msg == nil {
		return "", false
	}

	if msg.ViewOnceMessage != nil && msg.ViewOnceMessage.Message != nil {
		return GetText(msg.ViewOnceMessage.Message)
	} else if msg.ViewOnceMessageV2 != nil && msg.ViewOnceMessageV2.Message != nil {
		return GetText(msg.ViewOnceMessageV2.Message)
	} else if msg.ViewOnceMessageV2Extension != nil && msg.ViewOnceMessageV2Extension.Message != nil {
		return GetText(msg.ViewOnceMessageV2Extension.Message)
	} else if msg.EphemeralMessage != nil && msg.EphemeralMessage.Message != nil {
		return GetText(msg.EphemeralMessage.Message)
	} else if msg.EditedMessage != nil && msg.EditedMessage.Message != nil {
		return GetText(msg.EditedMessage.Message)
	} else if msg.DocumentWithCaptionMessage != nil && msg.DocumentWithCaptionMessage.Message != nil {
		return GetText(msg.DocumentWithCaptionMessage.Message)
	} else if msg.BotInvokeMessage != nil && msg.BotInvokeMessage.Message != nil {
		return GetText(msg.BotInvokeMessage.Message)
	} else if msg.DeviceSentMessage != nil && msg.DeviceSentMessage.Message != nil {
		return GetText(msg.DeviceSentMessage.Message)
	}

	if m := msg.GetConversation(); m != "" {
		return m, true
	} else if m := msg.GetExtendedTextMessage(); m != nil {
		return m.GetText(), true
	} else if m := msg.GetImageMessage(); m != nil {
		return m.GetCaption(), true
	} else if m := msg.GetVideoMessage(); m != nil {
		return m.GetCaption(), true
	} else if m := msg.GetDocumentMessage(); m != nil {
		return m.GetCaption(), true
	} else if m := msg.GetInteractiveResponseMessage(); m != nil {
		if nf := m.GetNativeFlowResponseMessage(); nf != nil {

			var payload map[string]interface{}
			paramsStr := nf.GetParamsJSON()

			fmt.Println("🤖 [DEBUG] Payload Tombol:", paramsStr)

			if err := json.Unmarshal([]byte(paramsStr), &payload); err == nil {
				if id, ok := payload["id"]; ok && id != nil {
					return fmt.Sprintf("%v", id), true
				}
			} else {

				var unescaped string
				if err := json.Unmarshal([]byte(paramsStr), &unescaped); err == nil {
					if err := json.Unmarshal([]byte(unescaped), &payload); err == nil {
						if id, ok := payload["id"]; ok && id != nil {
							return fmt.Sprintf("%v", id), true
						}
					}
				}
			}

			re := regexp.MustCompile(`"id"\s*:\s*"([^"]+)"`)
			matches := re.FindStringSubmatch(paramsStr)
			if len(matches) > 1 {
				return matches[1], true
			}
		}

		if bodyText := m.GetBody().GetText(); bodyText != "" {
			return bodyText, true
		}
		if nf := m.GetNativeFlowResponseMessage(); nf != nil {
			return nf.GetName(), true
		}
	} else if m := msg.GetListResponseMessage(); m != nil {
		if sel := m.GetSingleSelectReply(); sel != nil {
			if rowID := sel.GetSelectedRowID(); rowID != "" {
				return rowID, true
			}
		}
		return m.GetTitle(), true
	} else if m := msg.GetButtonsResponseMessage(); m != nil {

		if btnID := m.GetSelectedButtonID(); btnID != "" {
			return btnID, true
		}
		return m.GetSelectedDisplayText(), true
	} else if m := msg.GetTemplateButtonReplyMessage(); m != nil {

		if selID := m.GetSelectedID(); selID != "" {
			return selID, true
		}
		return m.GetSelectedDisplayText(), true
	}

	return "", false
}

func UpdatePushname(jid, name string) {
	if name == "" || jid == "" {
		return
	}
	existingName, ok := CachedName.Load(jid)
	if !ok || existingName.(string) != name {
		CachedName.Store(jid, name)
	}
}

func GetCachedName(jid string) string {
	value, ok := CachedName.Load(jid)
	if ok {
		return value.(string)
	}
	return strings.Split(jid, "@")[0]
}

func UnwrapAPI(body []byte, target interface{}) error {
	var wrapper struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return err
	}
	if len(wrapper.Data) > 0 {
		return json.Unmarshal(wrapper.Data, target)
	}
	return json.Unmarshal(body, target)
}
