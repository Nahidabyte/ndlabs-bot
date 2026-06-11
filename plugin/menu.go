package plugin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
	"wa-bot/lib"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func init() {
	RegisterPlugin(Cmd{
		Name:     "menu",
		Category: "main",
		Alias:    []string{"help", "h"},
		Desc:     "Menampilkan daftar perintah bot dengan desain premium",
		Exec:     menu,
	})
}

func menu(ctx *CommandContext) {

	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		loc = time.FixedZone("WIB", 7*3600)
	}
	currentTime := time.Now().In(loc)
	timeStr := currentTime.Format("15:04:05")
	dateStr := currentTime.Format("02/01/2006")

	greeting := "Selamat Malam"
	hour := currentTime.Hour()
	if hour >= 5 && hour < 11 {
		greeting = "Selamat Pagi"
	} else if hour >= 11 && hour < 15 {
		greeting = "Selamat Siang"
	} else if hour >= 15 && hour < 18 {
		greeting = "Selamat Sore"
	}

	userName := ctx.Msg.PushName
	if userName == "" {
		userName = "User"
	}

	sb := strings.Builder{}

	botName := os.Getenv("BOT_NAME")
	if botName == "" {
		botName = "ND LABS BOT"
	}

	requestedCategory := ""
	if len(ctx.Args) > 0 {
		requestedCategory = strings.ToLower(ctx.Args[0])
	}

	categoryMap := make(map[string][]string)
	for _, p := range ctx.Manager.GetAllItems() {
		cat := strings.ToLower(p.GetCategory())
		if cat == "hidden" {
			continue
		}
		if cat == "" {
			cat = "other"
		}
		for _, cmd := range p.GetCommands() {
			if cmd != "" {
				categoryMap[cat] = append(categoryMap[cat], cmd)
			}
		}
	}

	for cat, cmds := range categoryMap {
		sort.Strings(cmds)
		unique := make([]string, 0, len(cmds))
		for i, cmd := range cmds {
			if i == 0 || cmd != cmds[i-1] {
				unique = append(unique, cmd)
			}
		}
		categoryMap[cat] = unique
	}

	sb.WriteString(fmt.Sprintf("┏━━〔 *%s* 〕━━┓\n┃\n", strings.ToUpper(botName)))
	sb.WriteString(fmt.Sprintf("┃ 👤 *User:* %s\n", userName))
	sb.WriteString(fmt.Sprintf("┃ 🕒 *Waktu:* %s\n", timeStr))
	sb.WriteString(fmt.Sprintf("┃ 📅 *Tanggal:* %s\n", dateStr))
	sb.WriteString("┃ 🛠️ *Prefix:* '.'\n┃\n┗━━━━━━━━━━━━━━━━━━━━┛\n\n")
	sb.WriteString(fmt.Sprintf("👋 %s, *%s*!\n\n", greeting, userName))

	prefix := "."
	if len(ctx.Msg.Text) > 0 {
		prefix = string(ctx.Msg.Text[0])
	}

	footer := "ND Labs © 2026\nGunakan " + prefix + "help <perintah> untuk detail"
	thumbnailURL := "https://images.unsplash.com/photo-1614850523296-d8c1af93d400?q=50&w=200&h=200&auto=format&fit=crop&fm=jpg"

	var locMsg *waE2E.LocationMessage
	var thumbData []byte
	resp, err := http.Get(thumbnailURL)
	if err == nil {
		defer resp.Body.Close()
		thumbData, _ = io.ReadAll(resp.Body)
		locMsg = &waE2E.LocationMessage{
			DegreesLatitude:  proto.Float64(0),
			DegreesLongitude: proto.Float64(0),
			Name:             proto.String(strings.ToUpper(botName)),
			Address:          proto.String("ND LABS"),
			JPEGThumbnail:    thumbData,
		}
	}

	header := &lib.InteractiveHeader{}
	if locMsg != nil {
		header.Location = locMsg
	}

	var buttons []*lib.ButtonConfig

	if requestedCategory == "" {
		sb.WriteString("Silakan klik tombol di bawah untuk memilih kategori menu yang tersedia.\n")

		type Row struct {
			Title       string `json:"title"`
			ID          string `json:"id"`
			Description string `json:"description"`
		}
		type Section struct {
			Title string `json:"title"`
			Rows  []Row  `json:"rows"`
		}

		var rows []Row
		rows = append(rows, Row{
			Title:       "Semua Perintah",
			ID:          prefix + "menu all",
			Description: "Tampilkan seluruh perintah bot",
		})

		orderedCats := []string{"main", "general", "group", "tools", "media", "downloader", "economy", "info", "owner", "developer", "other", "ai"}
		catSet := make(map[string]bool)
		for _, c := range orderedCats {
			catSet[c] = true
		}

		var allCats []string
		allCats = append(allCats, orderedCats...)
		for cat := range categoryMap {
			if !catSet[cat] {
				allCats = append(allCats, cat)
			}
		}

		for _, cat := range allCats {
			if cmds, ok := categoryMap[cat]; ok && len(cmds) > 0 {
				rows = append(rows, Row{
					Title:       "Menu " + strings.Title(cat),
					ID:          prefix + "menu " + cat,
					Description: "Tampilkan perintah " + cat,
				})
			}
		}

		sections := []Section{
			{
				Title: "DAFTAR KATEGORI",
				Rows:  rows,
			},
		}

		sectionsJSON, _ := json.Marshal(sections)

		buttons = append(buttons, lib.SingleSelectButton("Pilih Kategori", string(sectionsJSON)))
	} else if requestedCategory == "all" {

		orderedCats := []string{"main", "general", "group", "tools", "media", "downloader", "economy", "info", "owner", "developer", "other", "ai"}

		for _, cat := range orderedCats {
			if cmds, ok := categoryMap[cat]; ok {
				sb.WriteString(fmt.Sprintf("┏━━〔 *%s* 〕\n", strings.ToUpper(cat)))
				for _, cmd := range cmds {
					sb.WriteString(fmt.Sprintf("┃ ◦ %s%s\n", prefix, cmd))
				}
				sb.WriteString("┗━━━━━━━━━━━━━━━━━━\n\n")
				delete(categoryMap, cat)
			}
		}

		var otherCats []string
		for cat := range categoryMap {
			otherCats = append(otherCats, cat)
		}
		sort.Strings(otherCats)
		for _, cat := range otherCats {
			cmds := categoryMap[cat]
			sb.WriteString(fmt.Sprintf("┏━━〔 *%s* 〕\n", strings.ToUpper(cat)))
			for _, cmd := range cmds {
				sb.WriteString(fmt.Sprintf("┃ ◦ %s%s\n", prefix, cmd))
			}
			sb.WriteString("┗━━━━━━━━━━━━━━━━━━\n\n")
		}

		buttons = append(buttons, lib.QuickReplyButton("🔙 Kembali", prefix+"menu"))
	} else {
		if cmds, ok := categoryMap[requestedCategory]; ok {
			sb.WriteString(fmt.Sprintf("┏━━〔 *%s MENU* 〕\n", strings.ToUpper(requestedCategory)))
			for _, cmd := range cmds {
				sb.WriteString(fmt.Sprintf("┃ ◦ %s%s\n", prefix, cmd))
			}
			sb.WriteString("┗━━━━━━━━━━━━━━━━━━\n")
		} else {
			sb.WriteString(fmt.Sprintf("❌ Kategori menu *%s* tidak ditemukan.\nKetik *%smenu* untuk melihat daftar kategori.", requestedCategory, prefix))
		}

		_, errBtn := ctx.Msg.SendButtonWithLocation(
			strings.ToUpper(botName),
			strings.ToUpper(requestedCategory)+" MENU",
			sb.String(),
			[]string{"🔙 Kembali", "🏠 Menu"},
			footer,
			thumbData,
		)
		if errBtn != nil {
			_, _ = ctx.Msg.Reply(sb.String() + "\n\n" + footer)
		}
		return
	}

	_, err = ctx.Msg.SendQuickReplyWithImage(
		header,
		sb.String(),
		buttons,
		footer,
	)
	if err != nil {
		_, _ = ctx.Msg.Reply(sb.String() + "\n\n" + footer)
	}
}
