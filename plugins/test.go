package plugins

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"wa-bot/lib"
	"wa-bot/plugin"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waAICommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

const VERSION = "4.2"

func init() {
	plugin.RegisterPlugin(plugin.Cmd{
		Name:     "test",
		Category: "developer",
		Alias:    []string{"tester"},
		Desc:     "Test berbagai jenis pesan bot lewat subcommand teks biasa",
		Exec:     testPlugin,
	})
}

func testPlugin(ctx *plugin.CommandContext) {
	if len(ctx.Args) == 0 {
		helpText := "Pilih jenis pesan test dengan mengetik:\n" +
			"!test <tipe>\n\n" +
			"Daftar tipe:\n" +
			"- text\n" +
			"- quoted\n" +
			"- button\n" +
			"- buttonquoted\n" +
			"- buttonimg\n" +
			"- buttonimgquoted\n" +
			"- quickreply\n" +
			"- quickreplyquoted\n" +
			"- urlbutton\n" +
			"- copybutton\n" +
			"- callbutton\n" +
			"- carousel\n" +
			"- carouselimg\n" +
			"- carouselquoted\n" +
			"- adreply\n" +
			"- media\n" +
			"- contact\n" +
			"- poll\n" +
			"- list\n" +
			"- sticker\n" +
			"- airich\n" +
			"- hydrated\n" +
			"- all\n\n" +
			"Contoh: !test carouselimg"
		ctx.Msg.Reply(helpText)
		return
	}

	switch strings.ToLower(ctx.Args[0]) {
	case "text":
		ctx.Msg.Reply("Ini adalah pesan teks biasa dari test command.")
	case "quoted":
		ctx.Msg.ReplyQuoted("Ini adalah pesan teks dengan quote dari test command.")
	case "button":
		ctx.Msg.SendButtonMessage(
			"Contoh pesan tombol:",
			[]string{"A", "B", "C"},
			"Test tombol",
		)
	case "buttonquoted":
		ctx.Msg.SendButtonQuoted(
			"Contoh pesan tombol dengan quote:",
			[]string{"A", "B", "C"},
			"Test tombol reply",
		)
	case "buttonimg":
		ctx.Msg.SendButtonWithImage(
			"Contoh tombol dengan gambar:",
			[]string{"A", "B", "C"},
			"Test tombol+gambar",
			"https://images.pexels.com/photos/414612/pexels-photo-414612.jpeg",
		)
	case "buttonimgquoted":
		ctx.Msg.SendButtonWithImageQuoted(
			"Contoh tombol dengan gambar + quote:",
			[]string{"A", "B", "C"},
			"Test tombol+gambar reply",
			"https://images.pexels.com/photos/414612/pexels-photo-414612.jpeg",
		)
	case "quickreply":
		ctx.Msg.SendQuickReply(
			"Contoh quick reply:",
			[]*lib.ButtonConfig{
				lib.QuickReplyButton("Pilih A", "A"),
				lib.QuickReplyButton("Pilih B", "B"),
			},
			"Footer quick reply",
		)
	case "quickreplyquoted":
		ctx.Msg.SendQuickReplyQuoted(
			"Contoh quick reply dengan quote:",
			[]*lib.ButtonConfig{
				lib.QuickReplyButton("Pilih A", "A"),
				lib.QuickReplyButton("Pilih B", "B"),
			},
			"Footer quick reply",
		)
	case "urlbutton":
		ctx.Msg.SendURLButtons(
			"Contoh URL button:",
			[]*lib.ButtonConfig{
				lib.URLButton("Buka Website", "https://example.com"),
			},
			"Footer URL button",
		)
	case "copybutton":
		ctx.Msg.SendCopyButtons(
			"Contoh copy button:",
			[]*lib.ButtonConfig{
				lib.CopyButton("Salin Kode", "TEST123"),
			},
			"Footer copy button",
		)
	case "callbutton":
		ctx.Msg.SendCallButtons(
			"Contoh call button:",
			[]*lib.ButtonConfig{
				lib.CallButton("Telepon", "+628123456789"),
			},
			"Footer call button",
		)
	case "carousel":
		ctx.Msg.SendCarousel(
			"Contoh carousel:",
			[]lib.CarouselCard{
				{
					Header: &lib.InteractiveHeader{Title: "Card 1"},
					Body:   "Deskripsi card 1",
					Footer: "Footer 1",
					Buttons: []*lib.ButtonConfig{
						lib.QuickReplyButton("Pilih 1", "1"),
					},
				},
				{
					Header: &lib.InteractiveHeader{Title: "Card 2"},
					Body:   "Deskripsi card 2",
					Footer: "Footer 2",
					Buttons: []*lib.ButtonConfig{
						lib.QuickReplyButton("Pilih 2", "2"),
					},
				},
			},
		)
	case "carouselimg":
		ctx.Msg.SendCarouselWithImages(
			"Contoh carousel dengan gambar:",
			[]string{
				"https://images.pexels.com/photos/414612/pexels-photo-414612.jpeg",
				"https://images.pexels.com/photos/257360/pexels-photo-257360.jpeg",
			},
			[]string{"Deskripsi card 1", "Deskripsi card 2"},
			[][]*lib.ButtonConfig{
				{
					lib.QuickReplyButton("Pilih 1", "1"),
				},
				{
					lib.QuickReplyButton("Pilih 2", "2"),
				},
			},
		)
	case "carouselquoted":
		ctx.Msg.SendCarouselQuoted(
			"Contoh carousel quoted:",
			[]lib.CarouselCard{
				{
					Header: &lib.InteractiveHeader{Title: "Card 1"},
					Body:   "Deskripsi card 1",
					Footer: "Footer 1",
					Buttons: []*lib.ButtonConfig{
						lib.QuickReplyButton("Pilih 1", "1"),
					},
				},
				{
					Header: &lib.InteractiveHeader{Title: "Card 2"},
					Body:   "Deskripsi card 2",
					Footer: "Footer 2",
					Buttons: []*lib.ButtonConfig{
						lib.QuickReplyButton("Pilih 2", "2"),
					},
				},
			},
		)
	case "adreply":
		ctx.Msg.SendAdReply(
			"Contoh pesan AdReply.",
			"Test AdReply",
			"Ini adalah body AdReply",
			"https://images.pexels.com/photos/414612/pexels-photo-414612.jpeg",
			"https://example.com",
		)
	case "media":
		ctx.Msg.SendMessage(plugin.MessageOptions{
			Text:      "Contoh kirim media image",
			MediaURL:  "https://images.pexels.com/photos/414612/pexels-photo-414612.jpeg",
			MediaType: "image",
			Caption:   "Contoh caption media",
		})
	case "contact":
		ctx.Msg.SendContact("Testing Contact", "628123456789")
	case "poll":
		ctx.Msg.SendPoll(
			"Pilih salah satu:",
			[]string{"Opsi 1", "Opsi 2", "Opsi 3"},
			1,
		)
	case "list":
		ctx.Msg.SendListMessage(
			"Pilih menu:",
			"Gunakan list di bawah ini",
			[]lib.ListSection{
				{
					Title: "Daftar Menu",
					Rows: []lib.ListRow{
						{Title: "Menu 1", Description: "Deskripsi menu 1", ID: "menu1"},
						{Title: "Menu 2", Description: "Deskripsi menu 2", ID: "menu2"},
					},
				},
			},
			"Test list",
		)
	case "sticker":
		stickerURL := "https://upload.wikimedia.org/wikipedia/commons/8/8e/Example_webp_image.webp"
		data, err := fetchRemoteBytes(stickerURL)
		if err != nil {
			ctx.Msg.Reply(fmt.Sprintf("Gagal mengambil sticker: %v", err))
			return
		}
		ctx.Msg.SendSticker(data)
	case "airich":
		builder := lib.NewAIRichBuilder()
		builder.SetTitle("AI Rich Capabilities Test")
		builder.AddText("Shiroko is my bini:\n- Model 1: [Nixel|1279|825|83|15](https://cdn.ornzora.eu.cc/1ca0f9a4-a81f-498e-92e8-8a4c76abf1ef-FIORA.png)\n- Model 2: [Nixel|1429|1897|83|15](https://cdn.ornzora.eu.cc/a3a756f2-6bb8-4814-a024-c325524a2308-FIORA.png)\n\nCek sumber [Ini Link](https://google.com) dan kutipan <https://wikipedia.org>")
		builder.AddCode("javascript", "const bot = 'WaBot';\nconsole.log(bot);")
		builder.AddTable([][]string{
			{"Nama", "Umur", "Role"},
			{"Shiroko", "16", "Striker"},
			{"Kuroko", "18", "Mystic"},
		})
		builder.AddImage([]string{"https://cdn.ornzora.eu.cc/1ca0f9a4-a81f-498e-92e8-8a4c76abf1ef-FIORA.png"})
		builder.AddSource([][]string{
			{"https://google.com/favicon.ico", "https://google.com", "Google Search"},
			{"https://wikipedia.org/favicon.ico", "https://wikipedia.org", "Wikipedia"},
		})
		builder.AddReels([]lib.ReelItem{
			{
				Title:          "Video Shiroko",
				ProfileIconUrl: "https://cdn.ornzora.eu.cc/1ca0f9a4-a81f-498e-92e8-8a4c76abf1ef-FIORA.png",
				ThumbnailUrl:   "https://cdn.ornzora.eu.cc/1ca0f9a4-a81f-498e-92e8-8a4c76abf1ef-FIORA.png",
				VideoUrl:       "https://www.w3schools.com/html/mov_bbb.mp4",
			},
		})
		ctx.Msg.SendAIRichMessage(builder)
	case "hydrated":
		ctx.Msg.SendHydratedTemplate(
			"5073 adalah kode verifikasi Anda. Demi keamanan, jangan bagikan kode ini.",
			"Kedaluwarsa dalam 5 menit.",
			[]*waE2E.HydratedTemplateButton{
				{
					Index: proto.Uint32(0),
					HydratedButton: &waE2E.HydratedTemplateButton_UrlButton{
						UrlButton: &waE2E.HydratedTemplateButton_HydratedURLButton{
							DisplayText: proto.String("Salin Kode"),
							URL:         proto.String("https://www.whatsapp.com/otp/code/?otp_type=ZERO_TAP&cta_display_name=Isi+Otomatis&package_name=com.gojek.gopay.dev%2Ccom.gojek.gopay.dev%2Ccom.gojek.gopay%2Ccom.gojek.gopay.alpha%2Ccom.gojek.gopaymerchant&signature_hash=bBevWzGTwET%2C955CVVAqcdT%2CNUM71yJif3H%2CeI5W4f3BDKT%2CU4hoZi%2BkgMN&code_expiration_minutes=5&code=otp5073"),
						},
					},
				},
				{
					Index: proto.Uint32(1),
					HydratedButton: &waE2E.HydratedTemplateButton_QuickReplyButton{
						QuickReplyButton: &waE2E.HydratedTemplateButton_HydratedQuickReplyButton{
							DisplayText: proto.String("Saya tidak menerima kode"),
							ID:          proto.String("DID_NOT_REQUEST_CODE"),
						},
					},
				},
			},
		)
	case "all":

		sections := `[{
			"title": "Main Menu",
			"rows": [
				{"header": "🔥 HOT", "title": "Downloader", "description": "Download social media", "id": ".dl"},
				{"header": "⚡ FAST", "title": "AI Chat", "description": "Chat dengan AI", "id": ".ai"}
			]
		}]`
		var imageMsg *waE2E.ImageMessage
		if imgData, err := fetchRemoteBytes("https://cdn.ornzora.eu.cc/b57c0d1e-d7a6-4277-8739-8f6b1d9894e6-FIORA.jpg"); err == nil {
			if uploadResp, err := ctx.Msg.Socket.Client.Upload(ctx.Msg.Socket.Ctx, imgData, whatsmeow.MediaImage); err == nil {
				imageMsg = &waE2E.ImageMessage{
					URL:           proto.String(uploadResp.URL),
					DirectPath:    proto.String(uploadResp.DirectPath),
					MediaKey:      uploadResp.MediaKey,
					Mimetype:      proto.String("image/jpeg"),
					FileEncSHA256: uploadResp.FileEncSHA256,
					FileSHA256:    uploadResp.FileSHA256,
					FileLength:    proto.Uint64(uint64(len(imgData))),
				}
			}
		}

		ctx.Msg.SendQuickReplyWithImage(
			&lib.InteractiveHeader{
				Title:    "🚀 NIXCODE",
				Subtitle: "Interactive Message",
				Image:    imageMsg,
			},
			"Pilih menu di bawah",
			[]*lib.ButtonConfig{
				lib.QuickReplyButton("📦 Menu", ".menu"),
				lib.QuickReplyButton("👤 Profile", ".profile"),
				lib.URLButton("🌐 Website", "https://example.com"),
				lib.CopyButton("📋 Copy Code", "NIX-2026"),
				lib.SingleSelectButton("📚 Pilih Kategori", sections),
			},
			"© Nixel",
		)

		var thumbBytes []byte
		_, errBtn2 := ctx.Msg.SendButtonWithLocation(
			"🚀 NIXCODE",
			"Buttons Message",
			"Halo dunia",
			[]string{"📦 Menu", "👤 Profile"},
			"Footer Message",
			thumbBytes,
		)
		if errBtn2 != nil {
			ctx.Msg.Reply(fmt.Sprintf("❌ Error Test 2 (ButtonV2): %v", errBtn2))
		}

		ctx.Msg.SendCarouselWithImages(
			"🛍️ Product List",
			[]string{
				"https://cdn.ornzora.eu.cc/36df8c36-c74e-4dc2-bc03-87893f373cb4-FIORA.jpg",
				"https://cdn.ornzora.eu.cc/36df8c36-c74e-4dc2-bc03-87893f373cb4-FIORA.jpg",
			},
			[]string{"Burger terenak", "Pizza mozzarella"},
			[][]*lib.ButtonConfig{
				{lib.QuickReplyButton("🛒 Buy", ".buy burger")},
				{lib.QuickReplyButton("🛒 Buy", ".buy pizza")},
			},
		)

		builder := lib.NewAIRichBuilder()
		builder.SetTitle("🚀 NIXCODE")
		builder.AddText(`
# Halo Dunia
## NIXCODE
---
=={ Yellow Text }==
---
Ini hyperlink:
[Text] (url) 
[Google](https://google.com)
Ini auto citation:
[] (url) 
[](https://openai.com)
Ini LaTeX:
[Identifier|?Width|?Height|?Font_Height|?Padding] <url>
[Shiroko|1429|1897]<https://cdn.ornzora.eu.cc/a3a756f2-6bb8-4814-a024-c325524a2308-FIORA.png>
		`)
		builder.AddCode("javascript", "class Nixel {\n\tstatic hello() {\n\t\treturn 'Hello World';\n\t}\n}")
		builder.AddTable([][]string{
			{"Nama", "Role"},
			{"Nixel", "Developer"},
			{"Fiora Sylvie", "Assistant"},
		})
		builder.AddSource([][]string{
			{"https://cdn.ornzora.eu.cc/dc85c945-96f7-4d50-aaa4-1dff7249aaf4-FIORA.jpg", "https://github.com/ValdazGT/", "GitHub"},
			{"https://cdn.ornzora.eu.cc/dc85c945-96f7-4d50-aaa4-1dff7249aaf4-FIORA.jpg", "https://fiora.nixel.my.id/", "Fiora Sylvie"},
		})
		builder.AddImage([]string{
			"https://cdn.ornzora.eu.cc/d987ff9c-c16c-4f1e-a8d6-953e375f4aec-FIORA.jpg",
			"https://cdn.ornzora.eu.cc/db9578dd-01e4-47ba-8a14-4c20e2aa4f52-FIORA.jpg",
		})
		builder.AddReels([]lib.ReelItem{
			{
				Title:          "Nixel",
				ProfileIconUrl: "https://cdn.ornzora.eu.cc/4d2905ce-3707-4ec0-998a-68a3d851629f-FIORA.jpg",
				ThumbnailUrl:   "https://cdn.ornzora.eu.cc/d6b36500-3b7e-49ee-9123-52bb1bf106be-FIORA.jpg",
				VideoUrl:       "https://fiora.nixel.my.id/",
			},
			{
				Title:          "Nixel",
				ProfileIconUrl: "https://cdn.ornzora.eu.cc/4d2905ce-3707-4ec0-998a-68a3d851629f-FIORA.jpg",
				ThumbnailUrl:   "https://cdn.ornzora.eu.cc/fb402a04-3f96-49d1-b4e2-faebbbd4a22c-FIORA.jpg",
				VideoUrl:       "https://fiora.nixel.my.id/",
			},
		})
		ctx.Msg.SendAIRichMessage(builder)
	case "airichv2":
		aiCtxInfo := &waE2E.ContextInfo{
			ForwardingScore: proto.Uint32(1),
			IsForwarded:     proto.Bool(true),
			ForwardedAiBotMessageInfo: &waAICommon.ForwardedAIBotMessageInfo{
				BotJID: proto.String("0@bot"),
			},
			ForwardOrigin: waE2E.ContextInfo_META_AI.Enum(),
		}
		var waButtons []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton
		waButtons = append(waButtons, &waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
			Name:             proto.String("quick_reply"),
			ButtonParamsJSON: proto.String(`{"display_text":"✨ AI Button 1","id":".menu"}`),
		})
		waButtons = append(waButtons, &waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
			Name:             proto.String("quick_reply"),
			ButtonParamsJSON: proto.String(`{"display_text":"✨ AI Button 2","id":".ping"}`),
		})
		msg := &waE2E.Message{
			MessageContextInfo: &waE2E.MessageContextInfo{
				DeviceListMetadataVersion: proto.Int32(2),
				BotMetadata: &waAICommon.BotMetadata{
					MessageDisclaimerText: proto.String("NIXCODE AI Bypass"),
				},
			},
			ViewOnceMessageV2: &waE2E.FutureProofMessage{
				Message: &waE2E.Message{
					InteractiveMessage: &waE2E.InteractiveMessage{
						ContextInfo: aiCtxInfo,
						Header: &waE2E.InteractiveMessage_Header{
							HasMediaAttachment: proto.Bool(false),
							Title:              proto.String("🚀 NIXCODE AI"),
							Subtitle:           proto.String("AI + Buttons Bypass"),
						},
						Body: &waE2E.InteractiveMessage_Body{
							Text: proto.String("=={ Yellow Text }==\nIni adalah eksperimen AI + Buttons.\n[Google](https://google.com)"),
						},
						InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{
							NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
								Buttons:        waButtons,
								MessageVersion: proto.Int32(1),
							},
						},
					},
				},
			},
		}
		bypassNodes := lib.GetLumiBypassNodes()
		ctx.Msg.Socket.Client.SendMessage(ctx.Ctx, ctx.Msg.Info.Chat, msg, whatsmeow.SendRequestExtra{AdditionalNodes: &bypassNodes})
	default:
		helpText := fmt.Sprintf("Unknown test type: %s\nGunakan !test untuk melihat daftar tipe pesan.\n", ctx.Args[0]) +
			"Daftar tipe: text, quoted, button, buttonquoted, buttonimg, buttonimgquoted, adreply, media, contact, poll, list, sticker, airich, hydrated, all"
		ctx.Msg.Reply(helpText)
	}
}

func fetchRemoteBytes(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}
