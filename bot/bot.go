package bot

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	"wa-bot/database"
	"wa-bot/lib"
	"wa-bot/plugin"
)

type Bot struct {
	client        *whatsmeow.Client
	pluginManager *plugin.Manager
	db            *database.DB
	container     *sqlstore.Container
	messagePrefix string
	isRunning     bool

	msgCache map[string]*events.Message
	msgIDs   []string
	msgMutex sync.RWMutex

	antiLinkCache map[string]string
	antiLinkMutex sync.RWMutex
}

type Config struct {
	MessagePrefix string
	DeviceDir     string
	DbPath        string
}

func New(config Config) (*Bot, error) {

	db, err := database.Init(config.DbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	container, err := sqlstore.New(
		context.Background(),
		"sqlite",
		fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", config.DbPath),
		waLog.Stdout("SQLStore", "INFO", true),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create sqlstore: %w", err)
	}

	deviceStore, err := container.GetFirstDevice(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get device: %w", err)
	}

	baseLogger := waLog.Stdout("Client", "INFO", true)
	client := whatsmeow.NewClient(deviceStore, &lib.FilterLogger{Logger: baseLogger})

	return &Bot{
		client:        client,
		pluginManager: plugin.NewManager(),
		db:            db,
		container:     container,
		messagePrefix: config.MessagePrefix,
		isRunning:     false,
		msgCache:      make(map[string]*events.Message),
		msgIDs:        make([]string, 0, 1000),
		antiLinkCache: make(map[string]string),
	}, nil
}

func (b *Bot) GetPluginManager() *plugin.Manager {
	return b.pluginManager
}

func (b *Bot) GetDatabase() *database.DB {
	return b.db
}

func (b *Bot) GetContainer() *sqlstore.Container {
	return b.container
}

func (b *Bot) GetClient() *whatsmeow.Client {
	return b.client
}

func (b *Bot) Connect(ctx context.Context) error {

	if b.client.Store.ID == nil {
		log.Println("Memulai proses login...")

		if err := b.client.Connect(); err != nil {
			return fmt.Errorf("gagal terhubung ke websocket: %w", err)
		}

		fmt.Print("\n============================================\n")
		fmt.Print("Masukkan nomor WhatsApp bot (contoh: 628123456789): ")
		var phone string
		fmt.Scanln(&phone)
		phone = strings.TrimSpace(phone)

		code, err := b.client.PairPhone(ctx, phone, true, whatsmeow.PairClientChrome, "Chrome (Windows)")
		if err != nil {
			return fmt.Errorf("gagal mendapatkan pairing code: %w", err)
		}

		fmt.Printf("\n>>> PAIRING CODE KAMU: %s <<<\n", code)
		fmt.Println("Silakan buka notifikasi perangkat tertaut di WhatsApp HP kamu,")
		fmt.Println("pilih 'Tautkan dengan nomor telepon', lalu masukkan kode di atas.")
		fmt.Println("============================================")

		loginWait := make(chan struct{})
		handlerID := b.client.AddEventHandler(func(evt interface{}) {
			if _, ok := evt.(*events.PairSuccess); ok {
				log.Println("Pairing Code berhasil diverifikasi! Login sukses.")
				close(loginWait)
			}
		})

		<-loginWait
		b.client.RemoveEventHandler(handlerID)
		return nil
	}

	if err := b.client.Connect(); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	return nil
}

func (b *Bot) Start(ctx context.Context) error {

	if err := b.pluginManager.InitAll(ctx); err != nil {
		return fmt.Errorf("failed to initialize plugins: %w", err)
	}

	b.isRunning = true
	log.Println("Bot started successfully")

	b.client.AddEventHandler(func(evt interface{}) {
		b.handleEvent(b.client, evt)
	})

	return nil
}

func (b *Bot) Stop(ctx context.Context) error {
	b.isRunning = false

	if err := b.pluginManager.CloseAll(ctx); err != nil {
		log.Printf("Error closing plugins: %v", err)
	}

	if b.client.IsConnected() {
		b.client.Disconnect()
	}

	if err := b.db.Close(); err != nil {
		log.Printf("Error closing database: %v", err)
	}

	log.Println("Bot stopped")
	return nil
}

func (b *Bot) handleEvent(client *whatsmeow.Client, evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		b.HandleMessage(context.Background(), client, v)
	case *events.GroupInfo:
		b.HandleGroupInfo(context.Background(), client, v)
	case *events.CallOffer:
		b.logCallNode(v.Data, "CallOffer")
	case *events.CallOfferNotice:
		b.logCallNode(v.Data, "CallOfferNotice")
	case *events.UnknownCallEvent:
		b.logCallNode(v.Node, "UnknownCallEvent")
	case *events.CallTransport:
		b.logCallNode(v.Data, "CallTransport")
	}
}

func (b *Bot) logCallNode(node *waBinary.Node, eventType string) {
	if node == nil {
		return
	}
	f, err := os.OpenFile("/home/nopal/Documents/golang_botwa/group_call_logs.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	xmlString := node.String()
	logLine := fmt.Sprintf("[%s] %s:\n%s\n\n", time.Now().Format(time.RFC3339), eventType, xmlString)
	f.WriteString(logLine)
	fmt.Print(logLine)
}

func (b *Bot) HandleMessage(ctx context.Context, client *whatsmeow.Client, msg *events.Message) {

	if msg.Message != nil {
		b.msgMutex.Lock()
		b.msgCache[msg.Info.ID] = msg
		b.msgIDs = append(b.msgIDs, msg.Info.ID)

		if len(b.msgIDs) > 1000 {
			oldestID := b.msgIDs[0]
			delete(b.msgCache, oldestID)
			b.msgIDs = b.msgIDs[1:]
		}
		b.msgMutex.Unlock()

		if protocolMsg := msg.Message.GetProtocolMessage(); protocolMsg != nil {
			if protocolMsg.GetType() == waE2E.ProtocolMessage_REVOKE {
				targetID := protocolMsg.GetKey().GetID()
				if targetID != "" {
					b.handleAntiDelete(ctx, client, msg, targetID)
				}
				return
			}
		}
	}

	if msg.Info.IsFromMe {
		return
	}

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

	text, _ := lib.GetText(msg.Message)

	if msg.Info.IsGroup {
		groupID := msg.Info.Chat.String()

		_, errLog := b.db.GetConnection().Exec(
			"INSERT OR IGNORE INTO messages (id, group_id, from_jid, created_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)",
			msg.Info.ID, groupID, senderJIDStr,
		)
		if errLog != nil {

			_, _ = b.db.GetConnection().Exec(
				"INSERT INTO messages (group_id, from_jid, created_at) VALUES (?, ?, CURRENT_TIMESTAMP)",
				groupID, senderJIDStr,
			)
		}
	}

	if msg.Info.IsGroup {
		b.checkMediaNSFW(client, msg)
	}

	if text == "" {
		return
	}

	b.db.AddUser(senderJIDStr)
	if msg.Info.PushName != "" {
		b.db.UpdateUserName(senderJIDStr, msg.Info.PushName)
	}

	if !msg.Info.IsFromMe {
		levelUp, newLevel, err := b.db.AddUserXP(senderJIDStr, 5)
		if err == nil && levelUp {
			if msg.Info.IsGroup {
				settings, _ := b.db.GetGroupSettings(msg.Info.Chat.String())

				if settings != nil && settings.Leveling {
					congrats := fmt.Sprintf("🎉 *LEVEL UP!* 🎉\n\nSelamat @%s, kamu telah naik ke *Level %d*! 🚀", senderUserStr, newLevel)
					_, _ = client.SendMessage(ctx, msg.Info.Chat, &waE2E.Message{
						ExtendedTextMessage: &waE2E.ExtendedTextMessage{
							Text: proto.String(congrats),
							ContextInfo: &waE2E.ContextInfo{
								MentionedJID: []string{senderJIDStr},
							},
						},
					})
				}
			} else {

				congrats := fmt.Sprintf("🎉 *LEVEL UP!* 🎉\n\nSelamat, kamu telah naik ke *Level %d*! 🚀", newLevel)
				_ = b.SendMessage(ctx, client, actualSender, congrats)
			}
		}
	}

	if msg.Info.IsGroup && strings.ToLower(text) == "ok" {

		var quotedID string
		if ext := msg.Message.GetExtendedTextMessage(); ext != nil {
			if ctxInfo := ext.GetContextInfo(); ctxInfo != nil {
				quotedID = ctxInfo.GetStanzaID()
			}
		}

		if quotedID != "" {
			b.antiLinkMutex.Lock()
			originalText, exists := b.antiLinkCache[quotedID]
			b.antiLinkMutex.Unlock()

			if exists {

				isAdmin := false
				groupInfo, err := client.GetGroupInfo(ctx, msg.Info.Chat)
				if err == nil {
					for _, part := range groupInfo.Participants {
						if (part.IsAdmin || part.IsSuperAdmin) && part.JID.ToNonAD().String() == senderJIDStr {
							isAdmin = true
							break
						}
					}
				}

				if isAdmin {

					resendMsg := fmt.Sprintf("✅ *Pesan Dipulihkan oleh Admin*\n\n%s", originalText)
					_ = b.SendMessage(ctx, client, msg.Info.Chat, resendMsg)

					b.antiLinkMutex.Lock()
					delete(b.antiLinkCache, quotedID)
					b.antiLinkMutex.Unlock()
					return
				}
			}
		}
	}

	if msg.Info.IsGroup && text != "" {
		if b.checkAntiFeatures(ctx, client, msg, text) {
			return
		}
	}

	textLower := strings.ToLower(text)

	hasPrefix := b.messagePrefix != "" && strings.HasPrefix(text, b.messagePrefix)

	isKilluaChat := false
	if msg.Info.IsGroup {
		settings, _ := b.db.GetGroupSettings(msg.Info.Chat.String())

		if settings != nil && settings.Simi && strings.Contains(textLower, "killua") {
			isKilluaChat = true
		}
	} else {

		if client == b.client {
			isKilluaChat = true
		} else if strings.Contains(textLower, "killua") {
			isKilluaChat = true
		}
	}

	botJID := b.client.Store.ID.ToNonAD().String()
	if client != nil && client.Store != nil && client.Store.ID != nil {
		botJID = client.Store.ID.ToNonAD().String()
	}
	botSettings, _ := b.db.GetBotSettings(botJID)
	isOwner := false

	envOwners := os.Getenv("OWNER_NUMBER")
	senderNum := msg.Info.Sender.User
	altNum := msg.Info.SenderAlt.User

	if envOwners != "" {
		for _, o := range strings.Split(envOwners, ",") {
			if o != "" && (senderNum == o || altNum == o) {
				isOwner = true
				break
			}
		}
	}

	if !isOwner && botSettings != nil {
		owners := strings.Split(botSettings.Owners, ",")
		for _, o := range owners {
			ownerNum := strings.TrimSpace(o)

			if ownerNum != "" && (senderNum == ownerNum || altNum == ownerNum) {
				isOwner = true
				break
			}
		}
	}

	var command string
	var args []string
	var isOwnerNoPrefix bool

	if hasPrefix {
		textTrimmed := strings.TrimPrefix(text, b.messagePrefix)
		parts := strings.Fields(textTrimmed)
		if len(parts) > 0 {
			command = strings.ToLower(parts[0])
			args = parts[1:]
		}
	} else if isKilluaChat {
		command = "killua"
		args = strings.Fields(text)
	} else if isOwner {
		parts := strings.Fields(text)
		if len(parts) > 0 {
			command = strings.ToLower(parts[0])
			args = parts[1:]
			isOwnerNoPrefix = true
		}
	}

	if command == "" && !isKilluaChat {
		return
	}

	isRegistered := false
	if command != "" {
		for _, p := range b.pluginManager.GetAll() {
			for _, cmd := range p.GetCommands() {
				if strings.EqualFold(cmd, command) {
					isRegistered = true
					break
				}
			}
			if isRegistered {
				break
			}
		}
	}

	if !isRegistered && isKilluaChat {
		command = "killua"
		args = strings.Fields(text)
	} else if !isRegistered {
		if isOwnerNoPrefix {

			return
		}
		if hasPrefix {

			return
		}
	}

	if command == "" {
		return
	}

	pushName := msg.Info.PushName
	if pushName == "" {
		pushName = "No Name"
	}

	chatType := "Private"
	if msg.Info.IsGroup {
		chatType = "Group"
	}

	logPrefix := fmt.Sprintf("[%s]", chatType)
	if isOwner {
		logPrefix += " [OWNER]"
	}

	logMsg := text
	if logMsg == "" {
		logMsg = "<Media/Non-Text>"
	}

	colorReset := "\033[0m"
	colorCyan := "\033[36m"
	colorGreen := "\033[32m"
	colorYellow := "\033[33m"
	colorMagenta := "\033[35m"

	fmt.Printf("%s[%s]%s %s%s%s (%s%s%s) -> %s%s%s\n",
		colorCyan, chatType, colorReset,
		colorGreen, pushName, colorReset,
		colorYellow, msg.Info.Sender.User, colorReset,
		colorMagenta, logMsg, colorReset,
	)

	isJadibot := client != b.client

	if client.Store.ID != nil {
		botJID = client.Store.ID.ToNonAD().String()
	}

	var isPremJadibot bool
	if isJadibot && botJID != "" {
		user, _ := b.db.GetUser(botJID)
		if user != nil && user.IsPremium {
			isPremJadibot = true
		}

		if msg.Info.IsGroup {
			maxGroups := 2
			if isPremJadibot {
				maxGroups = 9
			}

			lib.JManager.Mu.Lock()
			groups := lib.JManager.ActiveGroups[botJID]
			found := false
			for _, g := range groups {
				if g == msg.Info.Chat.String() {
					found = true
					break
				}
			}
			if !found {
				if len(groups) < maxGroups {
					lib.JManager.ActiveGroups[botJID] = append(groups, msg.Info.Chat.String())
				} else {
					lib.JManager.Mu.Unlock()
					return
				}
			}
			lib.JManager.Mu.Unlock()
		}
	}

	if botSettings != nil && !isOwner {
		if botSettings.Mode == "self" {
			return
		} else if botSettings.Mode == "group_only" && !msg.Info.IsGroup {
			return
		} else if botSettings.Mode == "private_only" && msg.Info.IsGroup {
			return
		}
	}

	socket := lib.NewSocket(ctx, client)
	socket.DB = b.db
	socket.GetCachedMessage = func(id string) *events.Message {
		b.msgMutex.RLock()
		defer b.msgMutex.RUnlock()
		return b.msgCache[id]
	}
	socket.CacheMessage = func(id string, m *events.Message) {
		if id == "" || m == nil {
			return
		}
		b.msgMutex.Lock()
		defer b.msgMutex.Unlock()
		if _, exists := b.msgCache[id]; !exists {
			b.msgIDs = append(b.msgIDs, id)
		}
		b.msgCache[id] = m

		if len(b.msgIDs) > 1000 {
			oldestID := b.msgIDs[0]
			delete(b.msgCache, oldestID)
			b.msgIDs = b.msgIDs[1:]
		}
	}
	socket.OnError = func(errText string, m *lib.WAMessage) {
		var owners []string
		if envOwners := os.Getenv("OWNER_NUMBER"); envOwners != "" {
			owners = append(owners, strings.Split(envOwners, ",")...)
		}

		errBotJID := botJID
		if errBotJID == "" {
			errBotJID = b.client.Store.ID.ToNonAD().String()
		}
		errSettings, _ := b.db.GetBotSettings(errBotJID)
		if errSettings != nil {
			owners = append(owners, strings.Split(errSettings.Owners, ",")...)
		}

		seen := make(map[string]bool)
		for _, o := range owners {
			o = strings.TrimSpace(o)
			if o != "" && !seen[o] {
				seen[o] = true
				ownerJID := types.NewJID(o, types.DefaultUserServer)
				logMsg := fmt.Sprintf("⚠️ *SYSTEM ERROR LOG*\n\n*User:* @%s\n*Chat:* %s\n*Bot Reply:* %s", m.Sender.User, m.JID.String(), errText)
				_, _ = client.SendMessage(context.Background(), ownerJID, &waE2E.Message{
					ExtendedTextMessage: &waE2E.ExtendedTextMessage{
						Text: proto.String(logMsg),
						ContextInfo: &waE2E.ContextInfo{
							MentionedJID: []string{m.Sender.ToNonAD().String()},
						},
					},
				})
			}
		}
	}
	waMsg := lib.NewWAMessage(socket, msg, text)

	log.Printf("DEBUG: Executing command '%s' with args %v", command, args)

	for _, p := range b.pluginManager.GetAll() {
		for _, cmd := range p.GetCommands() {
			if strings.EqualFold(cmd, command) {

				if p.IsOwnerOnly() && !isOwner {
					_ = b.SendMessage(ctx, client, msg.Info.Chat, "❌ Maaf, fitur ini khusus untuk *Owner Bot*.")
					return
				}
				if p.IsGroupOnly() && !msg.Info.IsGroup {
					_ = b.SendMessage(ctx, client, msg.Info.Chat, "❌ Fitur ini hanya bisa digunakan di dalam *Grup*.")
					return
				}
				if p.IsPrivateOnly() && msg.Info.IsGroup {
					_ = b.SendMessage(ctx, client, msg.Info.Chat, "❌ Fitur ini hanya bisa digunakan di *Private Chat (PC)*.")
					return
				}

				if msg.Info.IsGroup && (p.IsAdminOnly() || p.IsBotAdmin()) {
					groupInfo, err := client.GetGroupInfo(ctx, msg.Info.Chat)
					if err == nil {
						isAdmin := false
						isBotAdmin := false

						botJIDFull := client.Store.ID.ToNonAD()
						botLID := client.Store.LID.ToNonAD()

						senderJID := msg.Info.Sender.ToNonAD()
						senderLID := msg.Info.SenderAlt.ToNonAD()

						for _, part := range groupInfo.Participants {
							if part.IsAdmin || part.IsSuperAdmin {
								if part.JID.User == senderJID.User ||
									(senderLID.User != "" && part.JID.User == senderLID.User) ||
									(part.LID.User != "" && (part.LID.User == senderJID.User || part.LID.User == senderLID.User)) {
									isAdmin = true
								}
								if part.JID.User == botJIDFull.User ||
									(botLID.User != "" && part.JID.User == botLID.User) ||
									(part.LID.User != "" && (part.LID.User == botJIDFull.User || part.LID.User == botLID.User)) {
									isBotAdmin = true
								}
							}
						}
						if p.IsAdminOnly() && !isAdmin && !isOwner {
							_ = b.SendMessage(ctx, client, msg.Info.Chat, "❌ Perintah ini khusus *Admin Grup*!")
							return
						}
						if p.IsBotAdmin() && !isBotAdmin {
							_ = b.SendMessage(ctx, client, msg.Info.Chat, "❌ Bot harus jadi *Admin* dulu!")
							return
						}
					}
				}

				limitCost := p.GetLimitCost()
				if limitCost > 0 && !isOwner {
					user, err := b.db.GetUser(senderJIDStr)
					if err == nil && user != nil {
						if !user.IsPremium && user.Limit < limitCost {
							_ = b.SendMessage(ctx, client, msg.Info.Chat, fmt.Sprintf("❌ Limit harian kamu habis atau tidak cukup (Sisa: %d). Butuh %d limit untuk perintah ini.\n\n_Limit di-reset kembali jadi 100 setiap pergantian hari. Beli Premium untuk bebas limit._", user.Limit, limitCost))
							return
						}
					}
				}

				go func() {
					resp, err := p.Execute(ctx, waMsg, args)
					if err != nil {
						waMsg.Reply(fmt.Sprintf("❌ Error: %v", err))
					} else if resp != "" {
						if isJadibot && !isPremJadibot && msg.Info.IsGroup {
							resp += "\n\n---\n_🤖 Pesan ini dikirim oleh bot clone (Jadibot). Owner utama tidak bertanggung jawab atas aktivitas bot ini. Beli premium untuk menghilangkan pesan ini._"
						}
						waMsg.Reply(resp)
					}

					if err == nil && limitCost > 0 && !isOwner {
						user, err := b.db.GetUser(senderJIDStr)
						if err == nil && user != nil && !user.IsPremium {
							b.db.ReduceUserLimit(senderJIDStr, limitCost)
						}
					}
				}()
				return
			}
		}
	}
}

func (b *Bot) SendMessage(ctx context.Context, client *whatsmeow.Client, jid types.JID, text string) error {
	msg := &waE2E.Message{
		Conversation: proto.String(text),
	}

	_, err := client.SendMessage(ctx, jid, msg)
	return err
}

func (b *Bot) HandleGroupInfo(ctx context.Context, client *whatsmeow.Client, info *events.GroupInfo) {
	if len(info.Join) == 0 && len(info.Leave) == 0 {
		return
	}

	settings, err := b.db.GetGroupSettings(info.JID.String())
	if err != nil || settings == nil {
		return
	}

	botID := client.Store.ID.ToNonAD().String()

	meta, _ := client.GetGroupInfo(ctx, info.JID)
	groupName := "Grup"
	groupDesc := ""
	if meta != nil {
		groupName = meta.Name
		groupDesc = meta.Topic
	} else if info.Name != nil && info.Name.Name != "" {
		groupName = info.Name.Name
	}
	if groupDesc == "" {
		groupDesc = "Tidak ada deskripsi"
	}

	if len(info.Join) > 0 && settings.Welcome {
		for _, jid := range info.Join {
			actualJID := jid
			if jid.Server == "lid" && meta != nil {
				for _, p := range meta.Participants {
					if p.LID.User == jid.User {
						actualJID = p.JID
						break
					}
				}
			}

			if actualJID.ToNonAD().String() == botID {
				continue
			}

			text := settings.WelcomeText
			text = strings.ReplaceAll(text, "@user", "@"+actualJID.User)
			text = strings.ReplaceAll(text, "@group", groupName)
			text = strings.ReplaceAll(text, "@desc", groupDesc)

			msg := &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text:        proto.String("🎉 *WELCOME* 🎉\n\n" + text),
				ContextInfo: &waE2E.ContextInfo{MentionedJID: []string{actualJID.String()}},
			}}
			_, _ = client.SendMessage(ctx, info.JID, msg)

		}
	}

	if len(info.Leave) > 0 && settings.Goodbye {
		for _, jid := range info.Leave {
			actualJID := jid
			if jid.Server == "lid" && meta != nil {
				for _, p := range meta.Participants {
					if p.LID.User == jid.User {
						actualJID = p.JID
						break
					}
				}
			}

			if actualJID.ToNonAD().String() == botID {
				continue
			}

			text := settings.GoodbyeText
			text = strings.ReplaceAll(text, "@user", "@"+actualJID.User)
			text = strings.ReplaceAll(text, "@group", groupName)
			text = strings.ReplaceAll(text, "@desc", groupDesc)

			msg := &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text:        proto.String("👋 *GOODBYE* 👋\n\n" + text),
				ContextInfo: &waE2E.ContextInfo{MentionedJID: []string{actualJID.String()}},
			}}
			_, _ = client.SendMessage(ctx, info.JID, msg)
		}
	}
}

func (b *Bot) handleAntiDelete(ctx context.Context, client *whatsmeow.Client, revokeEvent *events.Message, targetID string) {
	if !revokeEvent.Info.IsGroup {
		return
	}

	settings, err := b.db.GetGroupSettings(revokeEvent.Info.Chat.String())
	if err != nil || settings == nil || !settings.AntiDelete {
		return
	}

	b.msgMutex.RLock()
	origMsg, ok := b.msgCache[targetID]
	b.msgMutex.RUnlock()

	if !ok || origMsg == nil || origMsg.Message == nil {
		return
	}

	if origMsg.Info.IsFromMe {
		return
	}

	if revokeEvent.Info.IsFromMe {
		return
	}

	groupInfo, err := client.GetGroupInfo(ctx, revokeEvent.Info.Chat)
	if err == nil {
		revokerJID := revokeEvent.Info.Sender.ToNonAD()
		revokerLID := revokeEvent.Info.SenderAlt.ToNonAD()
		for _, part := range groupInfo.Participants {
			if part.IsAdmin || part.IsSuperAdmin {
				if part.JID.User == revokerJID.User ||
					(revokerLID.User != "" && part.JID.User == revokerLID.User) ||
					(part.LID.User != "" && (part.LID.User == revokerJID.User || part.LID.User == revokerLID.User)) {
					return
				}
			}
		}
	}

	senderJID := origMsg.Info.Sender.String()
	senderUser := origMsg.Info.Sender.User
	loc, _ := time.LoadLocation("Asia/Jakarta")
	timeStr := origMsg.Info.Timestamp.In(loc).Format("15:04:05 WIB")

	warningTxt := fmt.Sprintf("🚫 *ANTI DELETE TERDETEKSI*\n\nUser: @%s\nWaktu: %s\n\n_Pesan yang dihapus akan dikirim ulang di bawah ini:_ 👇", senderUser, timeStr)

	_, _ = client.SendMessage(ctx, revokeEvent.Info.Chat, &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: proto.String(warningTxt),
			ContextInfo: &waE2E.ContextInfo{
				MentionedJID: []string{senderJID},
			},
		},
	})

	msgToResend := proto.Clone(origMsg.Message).(*waE2E.Message)

	if msgToResend.GetExtendedTextMessage() != nil {
		msgToResend.GetExtendedTextMessage().ContextInfo = nil
	} else if msgToResend.GetImageMessage() != nil {
		msgToResend.GetImageMessage().ContextInfo = nil
	} else if msgToResend.GetVideoMessage() != nil {
		msgToResend.GetVideoMessage().ContextInfo = nil
	} else if msgToResend.GetAudioMessage() != nil {
		msgToResend.GetAudioMessage().ContextInfo = nil
	} else if msgToResend.GetDocumentMessage() != nil {
		msgToResend.GetDocumentMessage().ContextInfo = nil
	} else if msgToResend.GetStickerMessage() != nil {
		msgToResend.GetStickerMessage().ContextInfo = nil
	}

	_, err = client.SendMessage(ctx, revokeEvent.Info.Chat, msgToResend)
	if err != nil {
		log.Printf("Gagal mengirim ulang pesan anti-delete: %v", err)
	}
}

func (b *Bot) IsRunning() bool {
	return b.isRunning
}

func (b *Bot) checkAntiFeatures(ctx context.Context, client *whatsmeow.Client, msg *events.Message, text string) bool {
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

	settings, err := b.db.GetGroupSettings(msg.Info.Chat.String())
	if err != nil || settings == nil {
		return false
	}

	textLower := strings.ToLower(text)
	kickTarget := false
	reason := ""

	if settings.AntiLinkWa && (strings.Contains(textLower, "chat.whatsapp.com") || strings.Contains(textLower, "wa.me/")) {
		kickTarget = true
		reason = "Link WhatsApp"
	} else if settings.AntiLink && (strings.Contains(textLower, "http://") || strings.Contains(textLower, "https://") || strings.Contains(textLower, "www.")) {
		kickTarget = true
		reason = "Link Umum"
	} else if settings.AntiToxic {
		toxics := []string{"anjing", "babi", "bangsat", "kontol", "memek", "ngentot"}
		for _, t := range toxics {
			if strings.Contains(textLower, t) {
				kickTarget = true
				reason = "Kata Kasar/Toxic"
				break
			}
		}
	}

	if settings.AntiTagSW && !kickTarget {
		isGroupStatus := msg.Message.GetGroupStatusMessage() != nil || msg.Message.GetGroupStatusMessageV2() != nil
		if isGroupStatus {
			tagUsage := b.db.AddTagSWUsage(senderJIDStr, msg.Info.Chat.String())
			if tagUsage > settings.TagSWLimit {
				kickTarget = true
				reason = fmt.Sprintf("Batas Harian Tag SW (Maksimal %d kali/hari)", settings.TagSWLimit)
			}
		}
	}

	if !kickTarget {
		return false
	}

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
		return false
	}

	if isBotAdmin {
		revokeMsg := client.BuildRevoke(msg.Info.Chat, msg.Info.Sender, msg.Info.ID)
		_, _ = client.SendMessage(ctx, msg.Info.Chat, revokeMsg)

		if strings.HasPrefix(reason, "Batas Harian Tag SW") {
			warningCount := b.db.AddWarning(msg.Info.Chat.String(), senderJIDStr)
			if warningCount >= 5 {
				_, _ = client.UpdateGroupParticipants(ctx, msg.Info.Chat, []types.JID{actualSender}, whatsmeow.ParticipantChangeRemove)
				b.db.ResetWarnings(msg.Info.Chat.String(), senderJIDStr)
				warning := fmt.Sprintf("🚨 *ANTI TAG SW TERDETEKSI*\n\n@%s telah *dikeluarkan* karena melewati batas harian peringatan Tag SW (5/5).", senderUserStr)
				_, _ = client.SendMessage(ctx, msg.Info.Chat, &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String(warning), ContextInfo: &waE2E.ContextInfo{MentionedJID: []string{senderJIDStr}}}})
			} else {
				warning := fmt.Sprintf("🚨 *ANTI TAG SW TERDETEKSI*\n\n@%s, kamu telah mencapai batas %d kali tag SW/hari!\n\n⚠️ *Peringatan: %d/5*\n_Jika peringatan mencapai 5, kamu akan otomatis dikeluarkan._", senderUserStr, settings.TagSWLimit, warningCount)
				_, _ = client.SendMessage(ctx, msg.Info.Chat, &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String(warning), ContextInfo: &waE2E.ContextInfo{MentionedJID: []string{senderJIDStr}}}})
			}
		} else if reason == "Link WhatsApp" || reason == "Link Umum" {
			var adminMentions []string
			for _, part := range groupInfo.Participants {
				if part.IsAdmin || part.IsSuperAdmin {
					adminMentions = append(adminMentions, part.JID.ToNonAD().String())
				}
			}
			mentions := []string{senderJIDStr}
			mentions = append(mentions, adminMentions...)

			warning := fmt.Sprintf("🚨 *ANTI %s TERDETEKSI*\n\n@%s mengirim link yang dilarang!\n\n_Admin grup dapat me-reply pesan ini dengan mengetik *ok* untuk mengirim ulang pesan yang dihapus._", strings.ToUpper(reason), senderUserStr)
			resp, _ := client.SendMessage(ctx, msg.Info.Chat, &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String(warning), ContextInfo: &waE2E.ContextInfo{MentionedJID: mentions}}})

			if resp.ID != "" {
				b.antiLinkMutex.Lock()
				b.antiLinkCache[resp.ID] = text
				b.antiLinkMutex.Unlock()
			}
		} else {

			warnings := b.db.AddWarning(msg.Info.Chat.String(), senderJIDStr)
			if warnings >= 10 {
				_, _ = client.UpdateGroupParticipants(ctx, msg.Info.Chat, []types.JID{actualSender}, whatsmeow.ParticipantChangeRemove)
				b.db.ResetWarnings(msg.Info.Chat.String(), senderJIDStr)
				warning := fmt.Sprintf("🚨 *ANTI %s TERDETEKSI*\n\n@%s telah *dikeluarkan* dari grup karena mencapai batas maksimal pelanggaran (10/10 peringatan).", strings.ToUpper(reason), senderUserStr)
				_, _ = client.SendMessage(ctx, msg.Info.Chat, &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String(warning), ContextInfo: &waE2E.ContextInfo{MentionedJID: []string{senderJIDStr}}}})
			} else {
				warning := fmt.Sprintf("🚨 *ANTI %s TERDETEKSI*\n\n@%s, kamu melanggar aturan grup!\n\n⚠️ *Peringatan: %d/10*\n_Jika mencapai 10 peringatan, kamu akan otomatis dikeluarkan._", strings.ToUpper(reason), senderUserStr, warnings)
				_, _ = client.SendMessage(ctx, msg.Info.Chat, &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String(warning), ContextInfo: &waE2E.ContextInfo{MentionedJID: []string{senderJIDStr}}}})
			}
		}
	} else {
		warning := fmt.Sprintf("🚨 *ANTI %s TERDETEKSI*\n\n@%s mengirim %s. (Bot butuh akses admin untuk menghapus & kick!)", strings.ToUpper(reason), senderUserStr, reason)
		_, _ = client.SendMessage(ctx, msg.Info.Chat, &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String(warning), ContextInfo: &waE2E.ContextInfo{MentionedJID: []string{senderJIDStr}}}})
	}

	return true
}
