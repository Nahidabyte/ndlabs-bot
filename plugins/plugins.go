package plugins

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"wa-bot/database"
	"wa-bot/plugin"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func init() {
	PluginRegistry = append(PluginRegistry, func(m *plugin.Manager, db *database.DB) {
		m.Register(NewPingPlugin())
		m.Register(NewHelpPlugin(m))
	})
}

func getOSName() string {
	data, err := os.ReadFile("/etc/os-release")
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				return strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), `"`) + " (" + runtime.GOARCH + ")"
			}
		}
	}
	return strings.ToUpper(runtime.GOOS) + " (" + runtime.GOARCH + ")"
}

func getMemoryInfo() (total float64, used float64, percent float64) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0, 0
	}
	var memTotal, memAvailable float64
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		var val float64
		fmt.Sscanf(fields[1], "%f", &val)
		switch fields[0] {
		case "MemTotal:":
			memTotal = val
		case "MemAvailable:":
			memAvailable = val
		}
	}
	total = memTotal / 1024 / 1024
	used = (memTotal - memAvailable) / 1024 / 1024
	if total > 0 {
		percent = (used / total) * 100
	}
	return
}

func getCPULoad() float64 {
	data, err := os.ReadFile("/proc/loadavg")
	if err == nil {
		fields := strings.Fields(string(data))
		if len(fields) > 0 {
			var load float64
			fmt.Sscanf(fields[0], "%f", &load)
			cores := float64(runtime.NumCPU())
			if cores > 0 {
				percent := (load / cores) * 100
				if percent > 100 {
					percent = 100
				}
				return percent
			}
		}
	}
	return 0.0
}

type PingPlugin struct {
	*plugin.BasePlugin
	startTime time.Time
}

func NewPingPlugin() *PingPlugin {
	return &PingPlugin{
		BasePlugin: plugin.NewBasePlugin(
			"Ping",
			"1.0.0",
			"Bot Author",
			"main",
			"Responds to ping with system info",
		),
		startTime: time.Now(),
	}
}

func (p *PingPlugin) GetCommands() []string {
	return []string{"ping"}
}

func (p *PingPlugin) Execute(ctx context.Context, msg *plugin.Message, args []string) (string, error) {

	senderJID := msg.Sender.ToNonAD()
	if senderJID.Server == "lid" && !msg.RawMessage.Info.SenderAlt.IsEmpty() {
		senderJID = msg.RawMessage.Info.SenderAlt.ToNonAD()
	}
	senderNum := senderJID.User

	isOwner := false
	envOwners := os.Getenv("OWNER_NUMBER")
	for _, o := range strings.Split(envOwners, ",") {
		if o != "" && senderNum == strings.TrimSpace(o) {
			isOwner = true
			break
		}
	}

	if !isOwner {
		msg.Reply("❌ Perintah ini khusus untuk Owner Bot.")
		return "", nil
	}

	start := time.Now()

	res, err := msg.Reply("🚀 `Memulai Diagnostik Sistem...`")
	if err != nil {
		return "", err
	}

	elapsed := time.Since(start)

	frames := []string{
		"🔍 `Mengecek Jaringan...` [■□□□□]",
		"💻 `Mengecek CPU & RAM...` [■■□□□]",
		"⚙️ `Mengambil Data Runtime...` [■■■■□]",
	}

	for _, frame := range frames {
		time.Sleep(500 * time.Millisecond)
		editMsg := msg.Socket.Client.BuildEdit(msg.JID, res.ID, &waE2E.Message{Conversation: proto.String(frame)})
		_, _ = msg.Socket.Client.SendMessage(ctx, msg.JID, editMsg)
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	uptime := time.Since(p.startTime).Round(time.Second)

	adminStatus := "-"
	if msg.IsGroup {
		adminStatus = "❌ Bukan Admin"
		groupInfo, err := msg.Socket.Client.GetGroupInfo(ctx, msg.JID)
		if err == nil {
			botJID := msg.Socket.Client.Store.ID.ToNonAD()
			botLID := msg.Socket.Client.Store.LID.ToNonAD()
			for _, part := range groupInfo.Participants {
				if part.IsAdmin || part.IsSuperAdmin {
					if part.JID.User == botJID.User || (botLID.User != "" && part.JID.User == botLID.User) || (part.LID.User != "" && (part.LID.User == botJID.User || part.LID.User == botLID.User)) {
						adminStatus = "✅ Admin"
						break
					}
				}
			}
		}
	} else {
		adminStatus = "Pribadi (PC)"
	}

	cpuUsage := getCPULoad()
	ramTotal, ramUsed, ramPercent := getMemoryInfo()

	responseText := fmt.Sprintf("🏓 *P O N G !*\n"+
		"Terhubung dalam *%d ms*\n\n"+
		"📊 *NETWORK & STATUS*\n"+
		" ◦ *Uptime:* %s\n"+
		" ◦ *Admin:* %s\n\n"+
		"💻 *SERVER HARDWARE*\n"+
		" ◦ *OS:* %s\n\n"+
		" ◦ *CPU:* %d Cores\n"+
		"   └ Load: %.1f%%\n\n"+
		" ◦ *RAM:* %.1f GB\n"+
		"   └ Usage: %.1f GB (%.1f%%)\n\n"+
		"⚙️ *ENGINE RUNTIME*\n"+
		" ◦ *Golang:* %s\n"+
		" ◦ *Goroutines:* %d\n"+
		" ◦ *GC Cycles:* %d",
		elapsed.Milliseconds(),
		uptime.String(),
		adminStatus,
		getOSName(),
		runtime.NumCPU(),
		cpuUsage,
		ramTotal, ramUsed, ramPercent,
		runtime.Version(),
		runtime.NumGoroutine(),
		m.NumGC,
	)

	editMsg := msg.Socket.Client.BuildEdit(msg.JID, res.ID, &waE2E.Message{Conversation: proto.String(responseText)})
	_, _ = msg.Socket.Client.SendMessage(ctx, msg.JID, editMsg)

	return "", nil
}


type HelpPlugin struct {
	*plugin.BasePlugin
	manager *plugin.Manager
}

func NewHelpPlugin(manager *plugin.Manager) *HelpPlugin {
	return &HelpPlugin{
		BasePlugin: plugin.NewBasePlugin(
			"Help",
			"1.0.0",
			"Bot Author",
			"main",
			"Shows available commands",
		),
		manager: manager,
	}
}

func (p *HelpPlugin) GetCommands() []string {
	return []string{"help"}
}

func (p *HelpPlugin) Execute(ctx context.Context, msg *plugin.Message, args []string) (string, error) {

	commands := p.manager.ListCommands()

	var response strings.Builder
	response.WriteString("📋 *DAFTAR PERINTAH BOT*\n\n")

	for pluginName, cmds := range commands {
		response.WriteString(fmt.Sprintf("🔹 *%s*\n", pluginName))
		for _, cmd := range cmds {
			response.WriteString(fmt.Sprintf("  • .%s\n", cmd))
		}
		response.WriteString("\n")
	}

	botName := os.Getenv("BOT_NAME")
	if botName == "" {
		botName = "WhatsApp Bot"
	}

	thumbnail := os.Getenv("THUMBNAIL_URL")
	if thumbnail == "" {
		thumbnail = "https://app.nd-labs.dev/preview.jpg"
	}

	botNumber := os.Getenv("BOT_NUMBER")
	sourceURL := "https://github.com/Nahidabyte"
	if botNumber != "" {
		sourceURL = "https://wa.me/" + botNumber
	}

	msg.Socket.SendAdReply(msg.JID, response.String(), botName, "Daftar Menu Bot", thumbnail, sourceURL, msg.RawMessage)
	return "", nil
}

