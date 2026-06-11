package lib

import (
	"context"
	"log"
	"sync"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

type JadibotManager struct {
	Clients          map[string]*whatsmeow.Client
	ActiveGroups     map[string][]string
	Mu               sync.RWMutex
	Store            *sqlstore.Container
	MessageHandler   func(ctx context.Context, client *whatsmeow.Client, msg *events.Message)
	GroupInfoHandler func(ctx context.Context, client *whatsmeow.Client, info *events.GroupInfo)
}

var JManager *JadibotManager

func InitJadibot(store *sqlstore.Container) {
	JManager = &JadibotManager{
		Clients:      make(map[string]*whatsmeow.Client),
		ActiveGroups: make(map[string][]string),
		Store:        store,
	}
}

func (jm *JadibotManager) GetClient(senderJID string) *whatsmeow.Client {
	jm.Mu.RLock()
	defer jm.Mu.RUnlock()
	return jm.Clients[senderJID]
}

func (jm *JadibotManager) AddClient(senderJID string, client *whatsmeow.Client) {
	jm.Mu.Lock()
	defer jm.Mu.Unlock()
	jm.Clients[senderJID] = client
}

func (jm *JadibotManager) RemoveClient(senderJID string) {
	jm.Mu.Lock()
	defer jm.Mu.Unlock()
	if client, ok := jm.Clients[senderJID]; ok {
		if client.IsConnected() {
			client.Logout(context.Background())
		} else {
			if client.Store != nil {
				client.Store.Delete(context.Background())
			}
		}
		delete(jm.Clients, senderJID)
	}
}

func (jm *JadibotManager) CreateJadibot(ctx context.Context, senderJID string) (*whatsmeow.Client, error) {
	jm.Mu.Lock()
	defer jm.Mu.Unlock()

	if old, ok := jm.Clients[senderJID]; ok {
		old.Disconnect()
	}

	devices, _ := jm.Store.GetAllDevices(ctx)
	var deviceStore *store.Device
	for _, d := range devices {
		if d.ID != nil && d.ID.ToNonAD().String() == senderJID {
			deviceStore = d
			break
		}
	}

	if deviceStore == nil {
		deviceStore = jm.Store.NewDevice()
	}

	baseLogger := waLog.Stdout("Jadibot-"+senderJID, "ERROR", true)
	client := whatsmeow.NewClient(deviceStore, &FilterLogger{Logger: baseLogger})

	client.AddEventHandler(func(evt interface{}) {
		switch v := evt.(type) {
		case *events.Message:
			if jm.MessageHandler != nil {
				jm.MessageHandler(context.Background(), client, v)
			}
		case *events.GroupInfo:
			if jm.GroupInfoHandler != nil {
				jm.GroupInfoHandler(context.Background(), client, v)
			}
		}
	})

	jm.Clients[senderJID] = client
	return client, nil
}

func (jm *JadibotManager) RestoreSessions(ctx context.Context, mainBotJID string) {
	devices, err := jm.Store.GetAllDevices(ctx)
	if err != nil {
		log.Printf("Gagal mengambil devices untuk auto-restore: %v", err)
		return
	}

	for _, d := range devices {
		if d.ID == nil {
			continue
		}
		jid := d.ID.ToNonAD().String()

		if jid == mainBotJID || jid == "" {
			continue
		}

		log.Printf("Auto-restoring Jadibot session: %s", jid)

		client, err := jm.CreateJadibot(ctx, jid)
		if err != nil {
			continue
		}

		go func(c *whatsmeow.Client, j string) {
			if err := c.Connect(); err != nil {
				log.Printf("Gagal reconnect jadibot %s: %v", j, err)
			}
		}(client, jid)
	}
}
