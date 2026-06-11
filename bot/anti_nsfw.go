package bot

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"wa-bot/lib"

	"google.golang.org/protobuf/proto"
)

const nsfwAPIBase = "https://app.nd-labs.dev/api/ai/vision/"

const nsfwWarnLimit = 5

type visionResult struct {
	Label      string `json:"label"`
	Confidence int    `json:"confidence"`
	IsNSFW     bool   `json:"is_nsfw"`
	IsJomok    bool   `json:"is_jomok"`
	IsGay      bool   `json:"is_gay"`
}

func (r visionResult) positive() bool {
	return r.IsNSFW || r.IsJomok || r.IsGay || r.Confidence > 70
}

func (b *Bot) checkMediaNSFW(client *whatsmeow.Client, msg *events.Message) {
	if os.Getenv("VISION_API_KEY") == "" {
		return
	}

	if !msg.Info.IsGroup || msg.Message == nil {
		return
	}

	settings, err := b.db.GetGroupSettings(msg.Info.Chat.String())
	if err != nil || settings == nil {
		return
	}
	if !settings.AntiNSFW && !settings.AntiJomok && !settings.AntiGay {
		return
	}

	media, mediaType := extractVisualMedia(msg.Message)
	if media == nil {
		return
	}

	go b.processMediaNSFW(client, msg, media, mediaType,
		settings.AntiNSFW, settings.AntiJomok, settings.AntiGay)
}

func extractVisualMedia(msg *waE2E.Message) (whatsmeow.DownloadableMessage, string) {
	if msg == nil {
		return nil, ""
	}

	switch {
	case msg.ViewOnceMessage != nil && msg.ViewOnceMessage.Message != nil:
		return extractVisualMedia(msg.ViewOnceMessage.Message)
	case msg.ViewOnceMessageV2 != nil && msg.ViewOnceMessageV2.Message != nil:
		return extractVisualMedia(msg.ViewOnceMessageV2.Message)
	case msg.ViewOnceMessageV2Extension != nil && msg.ViewOnceMessageV2Extension.Message != nil:
		return extractVisualMedia(msg.ViewOnceMessageV2Extension.Message)
	case msg.EphemeralMessage != nil && msg.EphemeralMessage.Message != nil:
		return extractVisualMedia(msg.EphemeralMessage.Message)
	case msg.DocumentWithCaptionMessage != nil && msg.DocumentWithCaptionMessage.Message != nil:
		return extractVisualMedia(msg.DocumentWithCaptionMessage.Message)
	}

	if img := msg.GetImageMessage(); img != nil {
		return img, "image"
	}
	if stc := msg.GetStickerMessage(); stc != nil {
		return stc, "sticker"
	}
	if vid := msg.GetVideoMessage(); vid != nil {
		return vid, "video"
	}
	return nil, ""
}

func (b *Bot) processMediaNSFW(client *whatsmeow.Client, msg *events.Message, media whatsmeow.DownloadableMessage, mediaType string, checkNSFW, checkJomok, checkGay bool) {
	defer func() {

		_ = recover()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	data, err := client.Download(ctx, media)
	if err != nil || len(data) == 0 {
		return
	}

	frame, ferr := lib.ExtractFrameJPEG(data)
	if ferr != nil || len(frame) == 0 {

		if mediaType == "image" {
			frame = data
		} else {
			return
		}
	}

	var labels []string
	if checkNSFW {
		if res, ok := b.callVision(ctx, "check_nsfw", frame); ok && res.positive() {
			labels = append(labels, fmt.Sprintf("PORN (%d%%)", res.Confidence))
		}
	}
	if checkJomok {
		if res, ok := b.callVision(ctx, "check_jomok", frame); ok && res.positive() {
			labels = append(labels, fmt.Sprintf("JOMOK (%d%%)", res.Confidence))
		}
	}
	if checkGay {
		if res, ok := b.callVision(ctx, "check_gay", frame); ok && res.positive() {
			labels = append(labels, fmt.Sprintf("GAY (%d%%)", res.Confidence))
		}
	}

	if len(labels) == 0 {
		return
	}

	b.enforceNSFWViolation(ctx, client, msg, mediaType, strings.Join(labels, ", "))
}

func (b *Bot) callVision(ctx context.Context, endpoint string, jpg []byte) (visionResult, bool) {
	var out visionResult

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	fw, err := w.CreateFormFile("image", "frame.jpg")
	if err != nil {
		return out, false
	}
	if _, err := fw.Write(jpg); err != nil {
		return out, false
	}
	w.Close()

	req, err := http.NewRequestWithContext(ctx, "POST", nsfwAPIBase+endpoint, &body)
	if err != nil {
		return out, false
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("X-API-Key", os.Getenv("VISION_API_KEY"))

	httpClient := &http.Client{Timeout: 130 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return out, false
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return out, false
	}
	if err := lib.UnwrapAPI(respBytes, &out); err != nil {
		return out, false
	}
	return out, true
}

func (b *Bot) enforceNSFWViolation(ctx context.Context, client *whatsmeow.Client, msg *events.Message, mediaType, detail string) {
	actualSender := msg.Info.Sender.ToNonAD()
	if actualSender.Server == "lid" || actualSender.Server == "hidden" {
		if !msg.Info.SenderAlt.IsEmpty() {
			actualSender = msg.Info.SenderAlt.ToNonAD()
		} else if pn, err := client.Store.LIDs.GetPNForLID(ctx, actualSender); err == nil && !pn.IsEmpty() {
			actualSender = pn.ToNonAD()
		}
	}
	senderJIDStr := actualSender.String()
	senderUserStr := actualSender.User

	isAdmin, isBotAdmin := false, false
	groupInfo, err := client.GetGroupInfo(ctx, msg.Info.Chat)
	if err == nil {
		botJID, botLID := client.Store.ID.ToNonAD(), client.Store.LID.ToNonAD()
		senderJID, senderLID := msg.Info.Sender.ToNonAD(), msg.Info.SenderAlt.ToNonAD()
		for _, part := range groupInfo.Participants {
			if part.IsAdmin || part.IsSuperAdmin {
				if part.JID.User == senderJID.User || (senderLID.User != "" && part.JID.User == senderLID.User) || (part.LID.User != "" && (part.LID.User == senderJID.User || part.LID.User == senderLID.User)) {
					isAdmin = true
				}
				if part.JID.User == botJID.User || (botLID.User != "" && part.JID.User == botLID.User) || (part.LID.User != "" && (part.LID.User == botJID.User || part.LID.User == botLID.User)) {
					isBotAdmin = true
				}
			}
		}
	}

	if isAdmin {
		return
	}

	mediaLabel := map[string]string{
		"image":   "gambar",
		"sticker": "sticker",
		"video":   "video",
	}[mediaType]
	if mediaLabel == "" {
		mediaLabel = "media"
	}

	if !isBotAdmin {
		warning := fmt.Sprintf("🔞 *KONTEN TERLARANG TERDETEKSI*\n\n@%s mengirim %s yang terdeteksi: *%s*.\n(Bot butuh akses admin untuk menghapus & mengeluarkan pelanggar!)", senderUserStr, mediaLabel, detail)
		_, _ = client.SendMessage(ctx, msg.Info.Chat, &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String(warning), ContextInfo: &waE2E.ContextInfo{MentionedJID: []string{senderJIDStr}}}})
		return
	}

	revokeMsg := client.BuildRevoke(msg.Info.Chat, msg.Info.Sender, msg.Info.ID)
	_, _ = client.SendMessage(ctx, msg.Info.Chat, revokeMsg)

	warnings := b.db.AddWarning(msg.Info.Chat.String(), senderJIDStr)
	if warnings >= nsfwWarnLimit {
		_, _ = client.UpdateGroupParticipants(ctx, msg.Info.Chat, []types.JID{actualSender}, whatsmeow.ParticipantChangeRemove)
		b.db.ResetWarnings(msg.Info.Chat.String(), senderJIDStr)
		warning := fmt.Sprintf("🔞 *KONTEN TERLARANG TERDETEKSI*\n\n@%s telah *dikeluarkan* dari grup karena mencapai batas pelanggaran konten (%d/%d).\nTerdeteksi: *%s*", senderUserStr, nsfwWarnLimit, nsfwWarnLimit, detail)
		_, _ = client.SendMessage(ctx, msg.Info.Chat, &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String(warning), ContextInfo: &waE2E.ContextInfo{MentionedJID: []string{senderJIDStr}}}})
		return
	}

	warning := fmt.Sprintf("🔞 *KONTEN TERLARANG TERDETEKSI*\n\n@%s, %s kamu dihapus karena terdeteksi: *%s*.\n\n⚠️ *Peringatan: %d/%d*\n_Jika mencapai %d peringatan, kamu akan otomatis dikeluarkan._", senderUserStr, mediaLabel, detail, warnings, nsfwWarnLimit, nsfwWarnLimit)
	_, _ = client.SendMessage(ctx, msg.Info.Chat, &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String(warning), ContextInfo: &waE2E.ContextInfo{MentionedJID: []string{senderJIDStr}}}})
}
