package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"wa-bot/bot"
	"wa-bot/database"
	"wa-bot/lib"
	"wa-bot/plugins"

	"github.com/joho/godotenv"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

func main() {

	if err := godotenv.Load(".env.local"); err != nil {
		log.Println("No .env.local file found, using default flags")
	}

	prefix := flag.String("prefix", getEnv("PREFIX", "."), "Command prefix")
	deviceDir := flag.String("device", getEnv("DEVICE_DIR", "database/.device"), "Device directory")
	dbPath := flag.String("db", getEnv("DATABASE_PATH", "database/wa-bot.db"), "Database path")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting WhatsApp Bot...")

	botConfig := bot.Config{
		MessagePrefix: *prefix,
		DeviceDir:     *deviceDir,
		DbPath:        *dbPath,
	}

	waBot, err := bot.New(botConfig)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	pluginManager := waBot.GetPluginManager()

	plugins.RegisterAll(pluginManager, waBot.GetDatabase())

	lib.InitJadibot(waBot.GetContainer())

	log.Println("Plugins registered:")
	for _, p := range pluginManager.GetAll() {
		log.Printf("  • %s v%s - %s", p.GetName(), p.GetVersion(), p.GetDescription())
	}

	availableCommands := []string{}
	for _, p := range pluginManager.GetAll() {
		for _, cmd := range p.GetCommands() {
			if cmd != "" {
				availableCommands = append(availableCommands, cmd)
			}
		}
	}
	sort.Strings(availableCommands)
	log.Printf("Available commands: %s", strings.Join(availableCommands, ", "))

	var aiCmds []database.CommandInfo
	for _, p := range pluginManager.GetAll() {
		cmds := p.GetCommands()
		if len(cmds) > 0 {
			aiCmds = append(aiCmds, database.CommandInfo{
				Name:        cmds[0],
				Category:    p.GetCategory(),
				Description: p.GetDescription(),
			})
		}
	}
	if err := waBot.GetDatabase().SyncCommands(aiCmds); err != nil {
		log.Printf("Failed to sync commands to DB: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log.Println("Connecting to WhatsApp...")
	if err := waBot.Connect(ctx); err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}

	if err := waBot.Start(ctx); err != nil {
		log.Fatalf("Failed to start bot: %v", err)
	}

	lib.JManager.MessageHandler = waBot.HandleMessage
	lib.JManager.GroupInfoHandler = waBot.HandleGroupInfo

	mainBotID := ""
	if waBot.GetClient() != nil && waBot.GetClient().Store.ID != nil {
		mainBotID = waBot.GetClient().Store.ID.ToNonAD().String()
	}
	lib.JManager.RestoreSessions(ctx, mainBotID)

	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if waBot.GetClient() != nil && waBot.GetClient().IsConnected() {
					loc, _ := time.LoadLocation("Asia/Jakarta")
					now := time.Now().In(loc).Format("15:04")
					rows, err := waBot.GetDatabase().GetConnection().Query("SELECT id, mute_open, mute_close FROM groups WHERE mute_open = ? OR mute_close = ?", now, now)
					if err == nil {
						for rows.Next() {
							var id, muteOpen, muteClose string
							if err := rows.Scan(&id, &muteOpen, &muteClose); err == nil {
								jid, _ := types.ParseJID(id)
								if muteOpen == now {
									_ = waBot.GetClient().SetGroupAnnounce(ctx, jid, false)
									waBot.GetClient().SendMessage(ctx, jid, &waE2E.Message{Conversation: proto.String("⏰ Jadwal: " + now + "\n✅ Grup telah dibuka secara otomatis.")})

									nowHour := time.Now().Hour()
									if nowHour >= 0 && nowHour < 8 {
										audioData, err := os.ReadFile("assets/welcome_night.ogg")
										if err == nil {
											resp, err := waBot.GetClient().Upload(ctx, audioData, whatsmeow.MediaAudio)
											if err == nil {
												audioMsg := &waE2E.Message{AudioMessage: &waE2E.AudioMessage{
													URL:           proto.String(resp.URL),
													DirectPath:    proto.String(resp.DirectPath),
													MediaKey:      resp.MediaKey,
													Mimetype:      proto.String("audio/ogg; codecs=opus"),
													FileEncSHA256: resp.FileEncSHA256,
													FileSHA256:    resp.FileSHA256,
													FileLength:    proto.Uint64(resp.FileLength),
													Seconds:       proto.Uint32(2),
													Waveform:      []byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x34, 0x4d, 0x50, 0x4f, 0x42, 0x3b, 0x45, 0x4e, 0x51, 0x54, 0x54, 0x53, 0x52, 0x50, 0x52, 0x54, 0x53, 0x52, 0x4f, 0x4d, 0x42, 0x35, 0x30, 0x2d, 0x20, 0xc, 0x2, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
													PTT:           proto.Bool(true),
												}}
												_, _ = waBot.GetClient().SendMessage(ctx, jid, audioMsg)
											}
										}
									}
								} else if muteClose == now {
									_ = waBot.GetClient().SetGroupAnnounce(ctx, jid, true)
									waBot.GetClient().SendMessage(ctx, jid, &waE2E.Message{Conversation: proto.String("⏰ Jadwal: " + now + "\n🔒 Grup telah ditutup secara otomatis.")})
								}
							}
						}
						rows.Close()
					}
				}
			}
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	log.Println("Bot is running. Press Ctrl+C to stop.")
	log.Printf("Message prefix: %s", *prefix)
	log.Println("Available commands: ping, greet, help, info, calc")

	<-sigChan
	log.Println("\nShutting down...")

	if err := waBot.Stop(ctx); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}

	log.Println("Bot stopped successfully")
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
