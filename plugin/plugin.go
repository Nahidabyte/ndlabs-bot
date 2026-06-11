package plugin

import (
	"context"
	"fmt"
	"strings"

	"wa-bot/lib"

	"github.com/google/uuid"
	"go.mau.fi/whatsmeow/proto/waE2E"
)

type Message = lib.WAMessage

type QuotedMessage = lib.QuotedMessage

type CommandContext = lib.CommandContext

type Nopal = lib.Socket

type MessageOptions = lib.MessageOptions

type AdReplyOptions = lib.AdReplyOptions

type Cmd struct {
	Name     string
	Category string
	Alias    []string
	Desc     string
	Exec     func(ctx *CommandContext)
}

var FunctionalPlugins []Cmd

func RegisterPlugin(c Cmd) {
	FunctionalPlugins = append(FunctionalPlugins, c)
}

func GetEphemeralDuration(msg *Message) (uint32, bool) {
	if msg == nil || msg.Message == nil {
		return 0, false
	}

	var ctxInfo *waE2E.ContextInfo
	if m := msg.Message.GetExtendedTextMessage(); m != nil {
		ctxInfo = m.GetContextInfo()
	} else if m := msg.Message.GetImageMessage(); m != nil {
		ctxInfo = m.GetContextInfo()
	} else if m := msg.Message.GetVideoMessage(); m != nil {
		ctxInfo = m.GetContextInfo()
	} else if m := msg.Message.GetDocumentMessage(); m != nil {
		ctxInfo = m.GetContextInfo()
	}

	if ctxInfo != nil {
		if exp := ctxInfo.GetExpiration(); exp > 0 {
			return exp, true
		}
	}

	if msgCtx := msg.Message.GetMessageContextInfo(); msgCtx != nil {
		if exp := msgCtx.GetMessageAddOnDurationInSecs(); exp > 0 {
			return exp, true
		}
	}
	return 0, false
}

type Plugin interface {
	GetName() string
	GetVersion() string
	GetAuthor() string
	GetDescription() string
	GetCategory() string
	GetCommands() []string

	IsAdminOnly() bool
	IsOwnerOnly() bool
	IsGroupOnly() bool
	IsPrivateOnly() bool
	IsPremiumOnly() bool
	IsBotAdmin() bool
	IsNsfw() bool
	GetLimitCost() int
	GetMinLevel() int

	Execute(ctx context.Context, msg *Message, args []string) (response string, err error)

	Init(ctx context.Context) error
	Close(ctx context.Context) error
}

type BasePlugin struct {
	ID          string
	Name        string
	Version     string
	Author      string
	Category    string
	Description string

	AdminOnly   bool
	OwnerOnly   bool
	GroupOnly   bool
	PrivateOnly bool
	PremiumOnly bool
	BotAdmin    bool
	Nsfw        bool
	LimitCost   int
	MinLevel    int
}

func (p *BasePlugin) GetName() string {
	return p.Name
}

func (p *BasePlugin) GetVersion() string {
	return p.Version
}

func (p *BasePlugin) GetAuthor() string {
	return p.Author
}

func (p *BasePlugin) GetDescription() string {
	return p.Description
}

func (p *BasePlugin) GetCategory() string {
	return p.Category
}

func (p *BasePlugin) GetCommands() []string {
	return []string{}
}

func (p *BasePlugin) IsAdminOnly() bool {
	return p.AdminOnly
}

func (p *BasePlugin) IsOwnerOnly() bool {
	return p.OwnerOnly
}

func (p *BasePlugin) IsGroupOnly() bool {
	return p.GroupOnly
}

func (p *BasePlugin) IsPrivateOnly() bool {
	return p.PrivateOnly
}

func (p *BasePlugin) IsPremiumOnly() bool {
	return p.PremiumOnly
}

func (p *BasePlugin) IsBotAdmin() bool {
	return p.BotAdmin
}

func (p *BasePlugin) IsNsfw() bool {
	return p.Nsfw
}

func (p *BasePlugin) GetLimitCost() int {
	return p.LimitCost
}

func (p *BasePlugin) GetMinLevel() int {
	return p.MinLevel
}

func (p *BasePlugin) Execute(ctx context.Context, msg *Message, args []string) (string, error) {
	return "", nil
}

func (p *BasePlugin) Init(ctx context.Context) error {
	return nil
}

func (p *BasePlugin) Close(ctx context.Context) error {
	return nil
}

func NewBasePlugin(name, version, author, category, description string) *BasePlugin {
	return &BasePlugin{
		ID:          uuid.New().String(),
		Name:        name,
		Version:     version,
		Author:      author,
		Category:    category,
		Description: description,

		AdminOnly:   false,
		OwnerOnly:   false,
		GroupOnly:   false,
		PrivateOnly: false,
		PremiumOnly: false,
		BotAdmin:    false,
		Nsfw:        false,
		LimitCost:   0,
		MinLevel:    1,
	}
}

type FunctionalPluginWrapper struct {
	BasePlugin
	Cmd     Cmd
	DB      interface{}
	Manager *Manager
}

func (p *FunctionalPluginWrapper) GetCommands() []string {
	cmds := []string{p.Cmd.Name}
	cmds = append(cmds, p.Cmd.Alias...)
	return cmds
}

func (p *FunctionalPluginWrapper) Execute(ctx context.Context, m *Message, args []string) (string, error) {
	defer func() {
		if r := recover(); r != nil {
			m.Reply(fmt.Sprintf("❌ Error kritis terjadi pada sistem bot (Panic: %v)", r))
		}
	}()

	cmdCtx := &lib.CommandContext{
		Ctx:     m.Socket.Ctx,
		Client:  m.Socket.Client,
		Msg:     m,
		Manager: p.Manager,
		DB:      p.DB,
		Args:    args,
		RawArgs: strings.Join(args, " "),
	}
	p.Cmd.Exec(cmdCtx)
	return "", nil
}

func WrapFunctionalPlugins(manager *Manager, db interface{}) []Plugin {
	var plugins []Plugin
	for _, c := range FunctionalPlugins {
		base := NewBasePlugin(c.Name, "1.0.0", "System", c.Category, c.Desc)
		plugins = append(plugins, &FunctionalPluginWrapper{
			BasePlugin: *base,
			Cmd:        c,
			DB:         db,
			Manager:    manager,
		})
	}
	return plugins
}
