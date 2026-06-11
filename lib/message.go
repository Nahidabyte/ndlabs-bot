package lib

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type Socket struct {
	Ctx    context.Context
	Client *whatsmeow.Client
	DB     interface{}

	OnError func(errText string, m *WAMessage)

	GetCachedMessage func(id string) *events.Message

	CacheMessage func(id string, msg *events.Message)

	bizCache   map[types.JID]bool
	bizCacheMu sync.RWMutex
}

type QuotedMessage struct {
	StanzaID   string
	Sender     types.JID
	Text       string
	IsImage    bool
	IsVideo    bool
	IsAudio    bool
	IsDocument bool
	IsSticker  bool
	RawMessage *waE2E.Message
	Quoted     *QuotedMessage
}

type WAMessage struct {
	Socket     *Socket
	Ctx        context.Context
	Client     *whatsmeow.Client
	JID        types.JID
	To         types.JID
	Sender     types.JID
	From       types.JID
	Info       types.MessageInfo
	Message    *waE2E.Message
	RawMessage *events.Message
	Quoted     *QuotedMessage
	Text       string
	PushName   string
	IsGroup    bool
}

type CommandContext struct {
	Ctx     context.Context
	Client  *whatsmeow.Client
	Msg     *WAMessage
	Message *WAMessage
	DB      interface{}
	Manager PluginManager
	Args    []string
	RawArgs string
}

type PluginItem interface {
	GetCategory() string
	GetCommands() []string
}

type PluginManager interface {
	GetAllItems() []PluginItem
}

type AdReplyOptions struct {
	Title        string
	Body         string
	ThumbnailURL string
	SourceURL    string
}

type MessageOptions struct {
	Text      string
	Caption   string
	Footer    string
	Buttons   []string
	AdReply   *AdReplyOptions
	MediaURL  string
	MediaType string
	Quoted    *events.Message
}

type InteractiveHeader struct {
	Title    string
	Subtitle string
	Image    *waE2E.ImageMessage
	Video    *waE2E.VideoMessage
	Document *waE2E.DocumentMessage
	Location *waE2E.LocationMessage
}

type CarouselCard struct {
	Header  *InteractiveHeader
	Body    string
	Footer  string
	Buttons []*ButtonConfig
}

func GetLumiBypassNodes() []waBinary.Node {
	return []waBinary.Node{
		{
			Tag:   "biz",
			Attrs: waBinary.Attrs{},
			Content: []waBinary.Node{
				{
					Tag:   "interactive",
					Attrs: waBinary.Attrs{"type": "native_flow", "v": "1"},
					Content: []waBinary.Node{
						{Tag: "native_flow", Attrs: waBinary.Attrs{"v": "9", "name": "mixed"}},
					},
				},
			},
		},
	}
}

func GenerateNativeFlowParams(data map[string]string) string {
	if data == nil {
		return "{}"
	}
	result := "{"
	for i, k := range sortedKeys(data) {
		if i > 0 {
			result += ","
		}
		result += fmt.Sprintf(`"%s":"%s"`, k, data[k])
	}
	result += "}"
	return result
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}
func sanitizeJSON(input string) string {

	clean := strings.Map(func(r rune) rune {
		if r < 32 && r != 9 && r != 10 && r != 13 {
			return -1
		}
		return r
	}, input)

	var js map[string]interface{}
	if err := json.Unmarshal([]byte(clean), &js); err != nil {

		return clean
	}

	out, _ := json.Marshal(js)
	return string(out)
}

func BuildInteractiveHeader(header *InteractiveHeader) *waE2E.InteractiveMessage_Header {
	if header == nil {
		return nil
	}

	msgHeader := &waE2E.InteractiveMessage_Header{
		HasMediaAttachment: proto.Bool(false),
	}
	if header.Title != "" {
		msgHeader.Title = proto.String(header.Title)
	}
	if header.Subtitle != "" {
		msgHeader.Subtitle = proto.String(header.Subtitle)
	}

	switch {
	case header.Location != nil:
		msgHeader.Media = &waE2E.InteractiveMessage_Header_LocationMessage{
			LocationMessage: header.Location,
		}
		msgHeader.HasMediaAttachment = proto.Bool(true)
	case header.Image != nil:
		msgHeader.Media = &waE2E.InteractiveMessage_Header_ImageMessage{
			ImageMessage: header.Image,
		}
		msgHeader.HasMediaAttachment = proto.Bool(true)
	case header.Video != nil:
		msgHeader.Media = &waE2E.InteractiveMessage_Header_VideoMessage{
			VideoMessage: header.Video,
		}
		msgHeader.HasMediaAttachment = proto.Bool(true)
	case header.Document != nil:
		msgHeader.Media = &waE2E.InteractiveMessage_Header_DocumentMessage{
			DocumentMessage: header.Document,
		}
		msgHeader.HasMediaAttachment = proto.Bool(true)
	}

	if msgHeader.Title == nil && msgHeader.Subtitle == nil && !*msgHeader.HasMediaAttachment {
		return nil
	}

	return msgHeader
}

func QuickReplyButton(displayText, id string) *ButtonConfig {
	return &ButtonConfig{
		Name:             "quick_reply",
		ButtonParamsJson: fmt.Sprintf("{\"display_text\":\"%s\",\"id\":\"%s\"}", sanitizeJSON(displayText), sanitizeJSON(id)),
	}
}

func URLButton(displayText, url string) *ButtonConfig {
	return &ButtonConfig{
		Name:             "cta_url",
		ButtonParamsJson: fmt.Sprintf("{\"display_text\":\"%s\",\"url\":\"%s\"}", sanitizeJSON(displayText), sanitizeJSON(url)),
	}
}

func CopyButton(displayText, code string) *ButtonConfig {
	return &ButtonConfig{
		Name:             "cta_copy",
		ButtonParamsJson: fmt.Sprintf("{\"display_text\":\"%s\",\"copy_code\":\"%s\"}", sanitizeJSON(displayText), sanitizeJSON(code)),
	}
}

func CallButton(displayText, phoneNumber string) *ButtonConfig {
	return &ButtonConfig{
		Name:             "cta_call",
		ButtonParamsJson: fmt.Sprintf("{\"display_text\":\"%s\",\"phone_number\":\"%s\"}", sanitizeJSON(displayText), sanitizeJSON(phoneNumber)),
	}
}

func CatalogButton(businessPhoneNumber string) *ButtonConfig {
	return &ButtonConfig{
		Name:             "cta_catalog",
		ButtonParamsJson: fmt.Sprintf("{\"business_phone_number\":\"%s\"}", sanitizeJSON(businessPhoneNumber)),
	}
}

func ReminderButton(displayText string) *ButtonConfig {
	return &ButtonConfig{
		Name:             "cta_reminder",
		ButtonParamsJson: fmt.Sprintf("{\"display_text\":\"%s\"}", sanitizeJSON(displayText)),
	}
}

func CancelReminderButton(displayText string) *ButtonConfig {
	return &ButtonConfig{
		Name:             "cta_cancel_reminder",
		ButtonParamsJson: fmt.Sprintf("{\"display_text\":\"%s\"}", sanitizeJSON(displayText)),
	}
}

func AddressMessageButton(displayText string) *ButtonConfig {
	return &ButtonConfig{
		Name:             "address_message",
		ButtonParamsJson: fmt.Sprintf("{\"display_text\":\"%s\"}", sanitizeJSON(displayText)),
	}
}

func SendLocationButton(displayText string) *ButtonConfig {
	return &ButtonConfig{
		Name:             "send_location",
		ButtonParamsJson: fmt.Sprintf("{\"display_text\":\"%s\"}", sanitizeJSON(displayText)),
	}
}

func OpenWebViewButton(title, link string) *ButtonConfig {
	return &ButtonConfig{
		Name:             "open_webview",
		ButtonParamsJson: fmt.Sprintf("{\"title\":\"%s\",\"link\":\"%s\"}", sanitizeJSON(title), sanitizeJSON(link)),
	}
}

func MPMButton(productID string) *ButtonConfig {
	return &ButtonConfig{
		Name:             "mpm",
		ButtonParamsJson: fmt.Sprintf("{\"product_id\":\"%s\"}", sanitizeJSON(productID)),
	}
}

func PaymentTransactionDetailsButton(transactionID string) *ButtonConfig {
	return &ButtonConfig{
		Name:             "wa_payment_transaction_details",
		ButtonParamsJson: fmt.Sprintf("{\"transaction_id\":\"%s\"}", sanitizeJSON(transactionID)),
	}
}

func AutomatedGreetingCatalogButton(businessPhoneNumber, catalogProductID string) *ButtonConfig {
	return &ButtonConfig{
		Name:             "automated_greeting_message_view_catalog",
		ButtonParamsJson: fmt.Sprintf("{\"business_phone_number\":\"%s\",\"catalog_product_id\":\"%s\"}", sanitizeJSON(businessPhoneNumber), sanitizeJSON(catalogProductID)),
	}
}

func GalaxyMessageButton(flowToken, flowID string) *ButtonConfig {
	return &ButtonConfig{
		Name:             "galaxy_message",
		ButtonParamsJson: fmt.Sprintf("{\"flow_token\":\"%s\",\"flow_id\":\"%s\"}", sanitizeJSON(flowToken), sanitizeJSON(flowID)),
	}
}

func SingleSelectButton(title, sectionsJSON string) *ButtonConfig {
	return &ButtonConfig{
		Name:             "single_select",
		ButtonParamsJson: fmt.Sprintf("{\"title\":\"%s\",\"sections\":%s}", sanitizeJSON(title), sectionsJSON),
	}
}

func (s *Socket) uploadImage(imageURL string) (*waE2E.ImageMessage, error) {
	resp, err := http.Get(imageURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	uploaded, err := s.Client.Upload(s.Ctx, data, whatsmeow.MediaImage)
	if err != nil {
		return nil, err
	}

	return &waE2E.ImageMessage{
		URL:           proto.String(uploaded.URL),
		DirectPath:    proto.String(uploaded.DirectPath),
		MediaKey:      uploaded.MediaKey,
		Mimetype:      proto.String("image/jpeg"),
		FileSHA256:    uploaded.FileSHA256,
		FileEncSHA256: uploaded.FileEncSHA256,
		FileLength:    proto.Uint64(uploaded.FileLength),
	}, nil
}

func mustJSON(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}

type ListSection struct {
	Title string
	Rows  []ListRow
}

type ListRow struct {
	Title       string
	Description string
	ID          string
}

func NewSocket(ctx context.Context, client *whatsmeow.Client) *Socket {
	return &Socket{
		Ctx:      ctx,
		Client:   client,
		bizCache: make(map[types.JID]bool),
	}
}

func isAnimatedWebP(data []byte) bool {
	if len(data) < 20 {
		return false
	}
	if !bytes.Equal(data[0:4], []byte("RIFF")) || !bytes.Equal(data[8:12], []byte("WEBP")) {
		return false
	}
	return bytes.Contains(data, []byte("ANIM"))
}

func ParseQuoted(msg *events.Message) *QuotedMessage {
	if msg.Message == nil {
		return nil
	}

	var ctxInfo *waE2E.ContextInfo
	if m := msg.Message.GetExtendedTextMessage(); m != nil {
		ctxInfo = m.GetContextInfo()
	} else if m := msg.Message.GetImageMessage(); m != nil {
		ctxInfo = m.GetContextInfo()
	} else if m := msg.Message.GetVideoMessage(); m != nil {
		ctxInfo = m.GetContextInfo()
	} else if m := msg.Message.GetAudioMessage(); m != nil {
		ctxInfo = m.GetContextInfo()
	} else if m := msg.Message.GetDocumentMessage(); m != nil {
		ctxInfo = m.GetContextInfo()
	} else if m := msg.Message.GetStickerMessage(); m != nil {
		ctxInfo = m.GetContextInfo()
	}

	if ctxInfo == nil || ctxInfo.QuotedMessage == nil {
		return nil
	}

	sender, _ := types.ParseJID(ctxInfo.GetParticipant())

	quoted := &QuotedMessage{
		StanzaID:   ctxInfo.GetStanzaID(),
		Sender:     sender,
		RawMessage: ctxInfo.QuotedMessage,
	}

	if m := ctxInfo.QuotedMessage.GetConversation(); m != "" {
		quoted.Text = m
	} else if m := ctxInfo.QuotedMessage.GetExtendedTextMessage(); m != nil {
		quoted.Text = m.GetText()
	} else if m := ctxInfo.QuotedMessage.GetImageMessage(); m != nil {
		quoted.Text = m.GetCaption()
		quoted.IsImage = true
	} else if m := ctxInfo.QuotedMessage.GetVideoMessage(); m != nil {
		quoted.Text = m.GetCaption()
		quoted.IsVideo = true
	} else if m := ctxInfo.QuotedMessage.GetAudioMessage(); m != nil {
		quoted.IsAudio = true
	} else if m := ctxInfo.QuotedMessage.GetDocumentMessage(); m != nil {
		quoted.Text = m.GetCaption()
		quoted.IsDocument = true
	} else if m := ctxInfo.QuotedMessage.GetStickerMessage(); m != nil {
		quoted.IsSticker = true
	}

	innerMsg := &events.Message{
		Info:    types.MessageInfo{},
		Message: ctxInfo.QuotedMessage,
	}
	innerMsg.Info.Sender = sender
	quoted.Quoted = parseInnerQuoted(innerMsg)

	return quoted
}

func parseInnerQuoted(msg *events.Message) *QuotedMessage {
	if msg.Message == nil {
		return nil
	}

	var ctxInfo *waE2E.ContextInfo
	if m := msg.Message.GetExtendedTextMessage(); m != nil {
		ctxInfo = m.GetContextInfo()
	} else if m := msg.Message.GetImageMessage(); m != nil {
		ctxInfo = m.GetContextInfo()
	} else if m := msg.Message.GetVideoMessage(); m != nil {
		ctxInfo = m.GetContextInfo()
	} else if m := msg.Message.GetAudioMessage(); m != nil {
		ctxInfo = m.GetContextInfo()
	} else if m := msg.Message.GetDocumentMessage(); m != nil {
		ctxInfo = m.GetContextInfo()
	} else if m := msg.Message.GetStickerMessage(); m != nil {
		ctxInfo = m.GetContextInfo()
	}

	if ctxInfo == nil || ctxInfo.QuotedMessage == nil {
		return nil
	}

	sender, _ := types.ParseJID(ctxInfo.GetParticipant())

	quoted := &QuotedMessage{
		StanzaID:   ctxInfo.GetStanzaID(),
		Sender:     sender,
		RawMessage: ctxInfo.QuotedMessage,
	}

	if m := ctxInfo.QuotedMessage.GetConversation(); m != "" {
		quoted.Text = m
	} else if m := ctxInfo.QuotedMessage.GetExtendedTextMessage(); m != nil {
		quoted.Text = m.GetText()
	} else if m := ctxInfo.QuotedMessage.GetImageMessage(); m != nil {
		quoted.Text = m.GetCaption()
		quoted.IsImage = true
	} else if m := ctxInfo.QuotedMessage.GetVideoMessage(); m != nil {
		quoted.Text = m.GetCaption()
		quoted.IsVideo = true
	} else if m := ctxInfo.QuotedMessage.GetAudioMessage(); m != nil {
		quoted.IsAudio = true
	} else if m := ctxInfo.QuotedMessage.GetDocumentMessage(); m != nil {
		quoted.Text = m.GetCaption()
		quoted.IsDocument = true
	} else if m := ctxInfo.QuotedMessage.GetStickerMessage(); m != nil {
		quoted.IsSticker = true
	}

	return quoted
}

func NewWAMessage(socket *Socket, evt *events.Message, text string) *WAMessage {
	return &WAMessage{
		Socket:     socket,
		Ctx:        socket.Ctx,
		Client:     socket.Client,
		JID:        evt.Info.Chat,
		To:         evt.Info.Chat,
		Sender:     evt.Info.Sender,
		From:       evt.Info.Sender,
		Info:       evt.Info,
		Message:    evt.Message,
		RawMessage: evt,
		Quoted:     ParseQuoted(evt),
		Text:       text,
		PushName:   evt.Info.PushName,
		IsGroup:    evt.Info.IsGroup,
	}
}

func isCriticalError(text string) bool {
	if !strings.HasPrefix(text, "❌") {
		return false
	}
	lower := strings.ToLower(text)
	if strings.Contains(lower, "format") ||
		strings.Contains(lower, "gunakan") ||
		strings.Contains(lower, "reply pesan") ||
		strings.Contains(lower, "kirim/balas") ||
		strings.Contains(lower, "masukkan") ||
		strings.Contains(lower, "hanya untuk") ||
		strings.Contains(lower, "hanya bisa") {
		return false
	}
	return strings.Contains(lower, "gagal") || strings.Contains(lower, "error") ||
		strings.Contains(lower, "kritis") || strings.Contains(lower, "server") ||
		strings.Contains(lower, "panic") || strings.Contains(lower, "kesalahan")
}

func (m *WAMessage) RelayMessage(msg any, opts ...any) (whatsmeow.SendResponse, error) {
	return m.Socket.RelayMessage(m.JID, msg, opts...)
}

func (m *WAMessage) RelayJSON(jsonStr string) (whatsmeow.SendResponse, error) {
	return m.Socket.RelayMessage(m.JID, jsonStr)
}

func (m *WAMessage) Edit(msgID string, newText string) (whatsmeow.SendResponse, error) {
	return m.Socket.Client.SendMessage(context.Background(), m.JID, &waE2E.Message{
		ProtocolMessage: &waE2E.ProtocolMessage{
			Type: waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(),
			Key: &waCommon.MessageKey{
				FromMe:    proto.Bool(true),
				ID:        proto.String(msgID),
				RemoteJID: proto.String(m.JID.String()),
			},
			EditedMessage: &waE2E.Message{
				Conversation: proto.String(newText),
			},
			TimestampMS: proto.Int64(time.Now().UnixMilli()),
		},
	})
}

func (m *WAMessage) Delete(msgID string) (whatsmeow.SendResponse, error) {
	revokeMsg := m.Socket.Client.BuildRevoke(m.JID, *m.Socket.Client.Store.ID, msgID)
	return m.Socket.Client.SendMessage(context.Background(), m.JID, revokeMsg)
}

func (m *WAMessage) Reply(text string) (whatsmeow.SendResponse, error) {
	if isCriticalError(text) && m.Socket.OnError != nil {
		go m.Socket.OnError(text, m)
	}
	return m.Socket.SendText(m.JID, text, nil)
}

func (m *WAMessage) ReplyQuoted(text string) (whatsmeow.SendResponse, error) {
	if isCriticalError(text) && m.Socket.OnError != nil {
		go m.Socket.OnError(text, m)
	}
	return m.Socket.SendText(m.JID, text, m.buildQuoted())
}

func (m *WAMessage) SendButtonMessage(body string, buttons []string, footer string) (whatsmeow.SendResponse, error) {
	return m.Socket.SendButtonMessage(m.JID, body, buttons, footer, nil)
}

func (m *WAMessage) SendButtonQuoted(body string, buttons []string, footer string) (whatsmeow.SendResponse, error) {
	return m.Socket.SendButtonMessage(m.JID, body, buttons, footer, m.buildQuoted())
}

func (m *WAMessage) SendButtonWithImage(body string, buttons []string, footer string, imageURL string) (whatsmeow.SendResponse, error) {
	return m.Socket.SendButtonWithImage(m.JID, body, buttons, footer, imageURL, nil)
}

func (m *WAMessage) SendButtonWithImageQuoted(body string, buttons []string, footer string, imageURL string) (whatsmeow.SendResponse, error) {
	return m.Socket.SendButtonWithImage(m.JID, body, buttons, footer, imageURL, m.buildQuoted())
}

func (m *WAMessage) SendButtonWithLocation(title, subtitle, body string, buttons []string, footer string, thumbnail []byte) (whatsmeow.SendResponse, error) {
	return m.Socket.SendButtonWithLocation(m.JID, title, subtitle, body, buttons, footer, thumbnail, nil)
}

func (m *WAMessage) SendQuickReply(body string, buttons []*ButtonConfig, footer string) (whatsmeow.SendResponse, error) {
	return m.Socket.SendQuickReplyButton(m.JID, body, buttons, footer, nil)
}

func (m *WAMessage) SendQuickReplyQuoted(body string, buttons []*ButtonConfig, footer string) (whatsmeow.SendResponse, error) {
	return m.Socket.SendQuickReplyButton(m.JID, body, buttons, footer, m.buildQuoted())
}

func (m *WAMessage) SendQuickReplyWithImage(header *InteractiveHeader, body string, buttons []*ButtonConfig, footer string) (whatsmeow.SendResponse, error) {
	return m.Socket.SendQuickReplyWithImage(m.JID, header, body, buttons, footer, nil)
}

func (m *WAMessage) SendURLButtons(body string, buttons []*ButtonConfig, footer string) (whatsmeow.SendResponse, error) {
	return m.Socket.SendURLButton(m.JID, body, buttons, footer, nil)
}

func (m *WAMessage) SendCopyButtons(body string, buttons []*ButtonConfig, footer string) (whatsmeow.SendResponse, error) {
	return m.Socket.SendCopyButton(m.JID, body, buttons, footer, nil)
}

func (m *WAMessage) SendCallButtons(body string, buttons []*ButtonConfig, footer string) (whatsmeow.SendResponse, error) {
	return m.Socket.SendCallButton(m.JID, body, buttons, footer, nil)
}

func (m *WAMessage) SendCarousel(body string, cards []CarouselCard) (whatsmeow.SendResponse, error) {
	return m.Socket.SendCarousel(m.JID, body, cards, nil)
}

func (m *WAMessage) SendCarouselQuoted(body string, cards []CarouselCard) (whatsmeow.SendResponse, error) {
	return m.Socket.SendCarousel(m.JID, body, cards, m.buildQuoted())
}

func (m *WAMessage) SendCarouselWithImages(body string, imageURLs []string, cardBodies []string, cardButtons [][]*ButtonConfig) (whatsmeow.SendResponse, error) {
	return m.Socket.SendCarouselWithImages(m.JID, body, imageURLs, cardBodies, cardButtons, nil)
}

func (m *WAMessage) SendCollection(body string, bizJID string) (whatsmeow.SendResponse, error) {
	return m.Socket.SendCollectionMessage(m.JID, body, bizJID, nil)
}

func (m *WAMessage) SendAdReply(text, title, adBody, thumbnailURL, sourceURL string) (whatsmeow.SendResponse, error) {
	return m.Socket.SendAdReply(m.JID, text, title, adBody, thumbnailURL, sourceURL, nil)
}

func (m *WAMessage) SendMedia(url, mediaType, caption string) (whatsmeow.SendResponse, error) {
	return m.Socket.SendMedia(m.JID, url, mediaType, caption, nil)
}

func (m *WAMessage) SendMediaBytes(data []byte, mediaType, caption string) (whatsmeow.SendResponse, error) {
	return m.Socket.SendMediaBytes(m.JID, data, mediaType, caption, nil)
}

func (m *WAMessage) SendDocumentBytes(data []byte, fileName, mimetype, caption string) (whatsmeow.SendResponse, error) {
	return m.Socket.SendDocumentBytes(m.JID, data, fileName, mimetype, caption, nil)
}

func (m *WAMessage) SendContact(name, phoneNumber string) (whatsmeow.SendResponse, error) {
	return m.Socket.SendContact(m.JID, name, phoneNumber, nil)
}

func (m *WAMessage) SendPoll(question string, options []string, selectableCount int) (whatsmeow.SendResponse, error) {
	return m.Socket.SendPoll(m.JID, question, options, selectableCount)
}

func (m *WAMessage) SendSticker(data []byte) (whatsmeow.SendResponse, error) {
	return m.Socket.SendSticker(m.JID, data, nil)
}

func (m *WAMessage) SendMessage(opts MessageOptions) (whatsmeow.SendResponse, error) {
	return m.Socket.SendMessage(m.JID, opts)
}

func (m *WAMessage) SendListMessage(title, description string, sections []ListSection, footer string) (whatsmeow.SendResponse, error) {
	return m.Socket.SendListMessage(m.JID, title, description, sections, footer, nil)
}

func (m *WAMessage) Download() ([]byte, error) {

	if m.Quoted != nil && m.Quoted.RawMessage != nil {
		if data, err := m.downloadFromMessage(m.Quoted.RawMessage); err == nil {
			return data, nil
		}
	}

	return m.downloadFromMessage(m.Message)
}

func (m *WAMessage) React(emoji string) error {
	if m.RawMessage == nil {
		return fmt.Errorf("tidak ada pesan asli untuk direaksi")
	}
	reactMsg := m.Socket.Client.BuildReaction(m.JID, m.RawMessage.Info.Sender, m.RawMessage.Info.ID, emoji)
	_, err := m.Socket.Client.SendMessage(m.Socket.Ctx, m.JID, reactMsg)
	return err
}

func (m *WAMessage) DownloadQuoted() ([]byte, error) {
	if m.Quoted == nil || m.Quoted.RawMessage == nil {
		return nil, fmt.Errorf("tidak ada quoted message dengan media")
	}
	return m.downloadFromMessage(m.Quoted.RawMessage)
}

func (m *WAMessage) downloadFromMessage(msg *waE2E.Message) ([]byte, error) {
	if msg == nil {
		return nil, fmt.Errorf("pesan tidak mengandung media")
	}

	if img := msg.GetImageMessage(); img != nil {
		return m.Client.Download(m.Ctx, img)
	}
	if vid := msg.GetVideoMessage(); vid != nil {
		return m.Client.Download(m.Ctx, vid)
	}
	if aud := msg.GetAudioMessage(); aud != nil {
		return m.Client.Download(m.Ctx, aud)
	}
	if stc := msg.GetStickerMessage(); stc != nil {
		return m.Client.Download(m.Ctx, stc)
	}
	if doc := msg.GetDocumentMessage(); doc != nil {
		return m.Client.Download(m.Ctx, doc)
	}

	return nil, fmt.Errorf("tipe pesan tidak didukung untuk download")
}

func (m *WAMessage) buildQuoted() *events.Message {
	if m.Message == nil {
		return nil
	}
	return &events.Message{
		Info:    m.Info,
		Message: m.Message,
	}
}

func (s *Socket) IsBusiness(jid types.JID) bool {
	if jid.Server != types.DefaultUserServer {
		return false
	}

	s.bizCacheMu.RLock()
	if cached, ok := s.bizCache[jid]; ok {
		s.bizCacheMu.RUnlock()
		return cached
	}
	s.bizCacheMu.RUnlock()

	resp, err := s.Client.GetUserInfo(s.Ctx, []types.JID{jid})
	isBiz := false
	if err == nil && len(resp) > 0 {
		info, ok := resp[jid]
		isBiz = ok && info.VerifiedName != nil
	}

	s.bizCacheMu.Lock()
	s.bizCache[jid] = isBiz
	s.bizCacheMu.Unlock()

	return isBiz
}

func (s *Socket) SetBusinessCache(jid types.JID, isBusiness bool) {
	s.bizCacheMu.Lock()
	s.bizCache[jid] = isBusiness
	s.bizCacheMu.Unlock()
}

func (s *Socket) SendText(jid types.JID, text string, quoted *events.Message) (whatsmeow.SendResponse, error) {
	if quoted != nil {
		return s.Client.SendMessage(s.Ctx, jid, &waE2E.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text: proto.String(text),
				ContextInfo: &waE2E.ContextInfo{
					StanzaID:      &quoted.Info.ID,
					Participant:   proto.String(quoted.Info.Sender.String()),
					QuotedMessage: quoted.Message,
				},
			},
		})
	}
	return s.Client.SendMessage(s.Ctx, jid, &waE2E.Message{
		Conversation: proto.String(text),
	})
}

type FakeChat struct {
	Participant  string
	Text         string
	Quoted       *waE2E.Message
	Forwarded    bool
	ForwardScore uint32
}

func (s *Socket) FakeChat(participant, text string, forwarded bool) *FakeChat {
	return &FakeChat{Participant: participant, Text: text, Forwarded: forwarded}
}

func (s *Socket) relayRaw(jid types.JID, msg *waE2E.Message, extra ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
	return s.Client.SendMessage(s.Ctx, jid, msg, extra...)
}

func (s *Socket) RelayMessage(jid types.JID, msg any, opts ...any) (whatsmeow.SendResponse, error) {
	waMsg, err := toWAMessage(msg)
	if err != nil {
		return whatsmeow.SendResponse{}, err
	}

	var extra []whatsmeow.SendRequestExtra
	var fake *FakeChat
	nodesDecided := false

	for _, o := range opts {
		switch v := o.(type) {
		case nil:

		case whatsmeow.SendRequestExtra:
			extra = append(extra, v)
			nodesDecided = true
		case []waBinary.Node:
			nn := v
			extra = append(extra, whatsmeow.SendRequestExtra{AdditionalNodes: &nn})
			nodesDecided = true
		case *[]waBinary.Node:
			extra = append(extra, whatsmeow.SendRequestExtra{AdditionalNodes: v})
			nodesDecided = true
		case *FakeChat:
			fake = v
		case FakeChat:
			fc := v
			fake = &fc
		case bool:
			if v {
				bypass := GetLumiBypassNodes()
				extra = append(extra, whatsmeow.SendRequestExtra{AdditionalNodes: &bypass})
			}
			nodesDecided = true
		case string:
			switch strings.ToLower(strings.TrimSpace(v)) {
			case "bypass", "biz", "native_flow", "nodes", "interactive":
				bypass := GetLumiBypassNodes()
				extra = append(extra, whatsmeow.SendRequestExtra{AdditionalNodes: &bypass})
				nodesDecided = true
			case "none", "raw", "plain", "norich":
				nodesDecided = true
			}
		}
	}

	if fake != nil {
		s.applyFakeChat(waMsg, fake)
	}

	if !nodesDecided && isButtonMsg(waMsg) {
		bypass := GetLumiBypassNodes()
		extra = append(extra, whatsmeow.SendRequestExtra{AdditionalNodes: &bypass})
	}

	resp, err := s.relayRaw(jid, waMsg, extra...)

	if err == nil && s.CacheMessage != nil && resp.ID != "" &&
		s.Client != nil && s.Client.Store != nil && s.Client.Store.ID != nil {
		s.CacheMessage(resp.ID, &events.Message{
			Info: types.MessageInfo{
				ID: resp.ID,
				MessageSource: types.MessageSource{
					Chat:     jid,
					Sender:   *s.Client.Store.ID,
					IsFromMe: true,
				},
			},
			Message: waMsg,
		})
	}

	return resp, err
}

func toWAMessage(msg any) (*waE2E.Message, error) {
	switch v := msg.(type) {
	case *waE2E.Message:
		return v, nil
	case string:
		return parseRelayJSON(v)
	case []byte:
		return parseRelayJSON(string(v))
	default:
		return nil, fmt.Errorf("RelayMessage: tipe pesan tidak didukung (%T), pakai *waE2E.Message atau string JSON", msg)
	}
}

func parseRelayJSON(s string) (*waE2E.Message, error) {
	s = CleanRelayJSON(s)
	var msg waE2E.Message
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal([]byte(s), &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func CleanRelayJSON(s string) string {
	s = strings.TrimSpace(s)
	for _, fence := range []string{"```go", "```json", "```"} {
		s = strings.TrimPrefix(s, fence)
	}
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)

	if i := strings.Index(s, "`"); i >= 0 {
		if j := strings.LastIndex(s, "`"); j > i {
			return strings.TrimSpace(s[i+1 : j])
		}
	}

	for _, prefix := range []string{"nopal.RelayMessage(", "m.RelayJSON(", "RelayJSON(", "m.RelayMessage(", "RelayMessage("} {
		if strings.HasPrefix(s, prefix) {
			s = strings.TrimPrefix(s, prefix)
			s = strings.TrimSpace(s)
			s = strings.TrimSuffix(s, ")")
			s = strings.TrimSpace(s)
			break
		}
	}
	return strings.TrimSpace(s)
}

func isButtonMsg(msg *waE2E.Message) bool {
	if msg == nil {
		return false
	}
	return msg.GetInteractiveMessage() != nil || msg.GetButtonsMessage() != nil ||
		msg.GetTemplateMessage() != nil || msg.GetListMessage() != nil
}

func (s *Socket) applyFakeChat(msg *waE2E.Message, fc *FakeChat) {
	if msg == nil || fc == nil {
		return
	}
	ci := getOrInitContextInfo(msg)

	if fc.Participant != "" || fc.Text != "" || fc.Quoted != nil {
		participant := fc.Participant
		if participant == "" {
			participant = "0@s.whatsapp.net"
		}
		ci.Participant = proto.String(participant)
		ci.StanzaID = proto.String(s.Client.GenerateMessageID())
		if fc.Quoted != nil {
			ci.QuotedMessage = fc.Quoted
		} else {
			ci.QuotedMessage = &waE2E.Message{Conversation: proto.String(fc.Text)}
		}
	}

	if fc.Forwarded {
		score := fc.ForwardScore
		if score == 0 {
			score = 100
		}
		ci.IsForwarded = proto.Bool(true)
		ci.ForwardingScore = proto.Uint32(score)
	}
}

func getOrInitContextInfo(msg *waE2E.Message) *waE2E.ContextInfo {
	ensure := func(ci **waE2E.ContextInfo) *waE2E.ContextInfo {
		if *ci == nil {
			*ci = &waE2E.ContextInfo{}
		}
		return *ci
	}
	switch {
	case msg.ExtendedTextMessage != nil:
		return ensure(&msg.ExtendedTextMessage.ContextInfo)
	case msg.ImageMessage != nil:
		return ensure(&msg.ImageMessage.ContextInfo)
	case msg.VideoMessage != nil:
		return ensure(&msg.VideoMessage.ContextInfo)
	case msg.DocumentMessage != nil:
		return ensure(&msg.DocumentMessage.ContextInfo)
	case msg.AudioMessage != nil:
		return ensure(&msg.AudioMessage.ContextInfo)
	case msg.StickerMessage != nil:
		return ensure(&msg.StickerMessage.ContextInfo)
	case msg.InteractiveMessage != nil:
		return ensure(&msg.InteractiveMessage.ContextInfo)
	case msg.ButtonsMessage != nil:
		return ensure(&msg.ButtonsMessage.ContextInfo)
	case msg.ListMessage != nil:
		return ensure(&msg.ListMessage.ContextInfo)
	case msg.TemplateMessage != nil:
		return ensure(&msg.TemplateMessage.ContextInfo)
	case msg.Conversation != nil:

		msg.ExtendedTextMessage = &waE2E.ExtendedTextMessage{
			Text:        proto.String(msg.GetConversation()),
			ContextInfo: &waE2E.ContextInfo{},
		}
		msg.Conversation = nil
		return msg.ExtendedTextMessage.ContextInfo
	default:
		if msg.ExtendedTextMessage == nil {
			msg.ExtendedTextMessage = &waE2E.ExtendedTextMessage{ContextInfo: &waE2E.ContextInfo{}}
		}
		return ensure(&msg.ExtendedTextMessage.ContextInfo)
	}
}

func (s *Socket) SendMessage(jid types.JID, opts MessageOptions) (whatsmeow.SendResponse, error) {
	if opts.MediaURL != "" {
		caption := opts.Caption
		if caption == "" {
			caption = opts.Text
		}
		return s.SendMedia(jid, opts.MediaURL, opts.MediaType, caption, opts.Quoted)
	}
	if len(opts.Buttons) > 0 {
		return s.SendButtonMessage(jid, opts.Text, opts.Buttons, opts.Footer, opts.Quoted)
	}
	if opts.AdReply != nil {
		return s.SendAdReply(jid, opts.Text, opts.AdReply.Title, opts.AdReply.Body, opts.AdReply.ThumbnailURL, opts.AdReply.SourceURL, opts.Quoted)
	}

	if opts.Footer != "" {
		return s.sendTextWithFooter(jid, opts.Text, opts.Footer, opts.Quoted)
	}
	return s.SendText(jid, opts.Text, opts.Quoted)
}

func (s *Socket) sendTextWithFooter(jid types.JID, body string, footer string, quoted *events.Message) (whatsmeow.SendResponse, error) {
	interactiveMsg := &waE2E.InteractiveMessage{
		Body: &waE2E.InteractiveMessage_Body{
			Text: proto.String(body),
		},
		Footer: &waE2E.InteractiveMessage_Footer{
			Text: proto.String(footer),
		},
		InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{
			NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
				MessageVersion: proto.Int32(1),
				Buttons:        []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{},
			},
		},
	}

	if quoted != nil {
		interactiveMsg.ContextInfo = &waE2E.ContextInfo{
			StanzaID:      &quoted.Info.ID,
			Participant:   proto.String(quoted.Info.Sender.String()),
			QuotedMessage: quoted.Message,
		}
	}

	bypassNodes := GetLumiBypassNodes()
	return s.Client.SendMessage(s.Ctx, jid, &waE2E.Message{
		InteractiveMessage: interactiveMsg,
	}, whatsmeow.SendRequestExtra{AdditionalNodes: &bypassNodes})
}

func (s *Socket) SendButtonMessage(jid types.JID, body string, buttons []string, footer string, quoted *events.Message) (whatsmeow.SendResponse, error) {
	isBiz := s.IsBusiness(jid)

	if isBiz {
		return s.sendBusinessList(jid, body, buttons, footer, quoted)
	}

	return s.sendPersonalButtons(jid, body, buttons, footer, quoted)
}

func (s *Socket) sendPersonalButtons(jid types.JID, body string, buttons []string, footer string, quoted *events.Message) (whatsmeow.SendResponse, error) {
	var waButtons []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton
	for _, b := range buttons {

		displayText := b
		buttonID := b

		if len(b) > 0 && b[0] != '.' {
			buttonID = "." + b
		}
		waButtons = append(waButtons, &waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
			Name: proto.String("quick_reply"),
			ButtonParamsJSON: proto.String(fmt.Sprintf(`{"display_text":%s,"id":%s}`,
				mustJSON(displayText), mustJSON(buttonID))),
		})
	}

	interactiveMsg := &waE2E.InteractiveMessage{
		Header: &waE2E.InteractiveMessage_Header{
			Title:              proto.String("ND LABS BOT"),
			HasMediaAttachment: proto.Bool(false),
		},
		Body: &waE2E.InteractiveMessage_Body{
			Text: proto.String(body),
		},
		InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{
			NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
				Buttons:        waButtons,
				MessageVersion: proto.Int32(1),
			},
		},
	}

	if footer != "" {
		interactiveMsg.Footer = &waE2E.InteractiveMessage_Footer{
			Text: proto.String(footer),
		}
	}

	if quoted != nil {
		interactiveMsg.ContextInfo = &waE2E.ContextInfo{
			StanzaID:      &quoted.Info.ID,
			Participant:   proto.String(quoted.Info.Sender.String()),
			QuotedMessage: quoted.Message,
		}
	}

	bypassNodes := GetLumiBypassNodes()
	return s.Client.SendMessage(s.Ctx, jid, &waE2E.Message{
		InteractiveMessage: interactiveMsg,
	}, whatsmeow.SendRequestExtra{AdditionalNodes: &bypassNodes})
}

func (s *Socket) sendBusinessList(jid types.JID, body string, buttons []string, footer string, quoted *events.Message) (whatsmeow.SendResponse, error) {

	var rows []*waE2E.ListMessage_Row
	for i, b := range buttons {
		rows = append(rows, &waE2E.ListMessage_Row{
			Title: proto.String(b),
			RowID: proto.String(fmt.Sprintf("id-%d", i)),
		})
	}

	listMsg := &waE2E.ListMessage{
		Title:       proto.String(body),
		Description: proto.String(footer),
		ButtonText:  proto.String("Pilih Menu"),
		ListType:    waE2E.ListMessage_SINGLE_SELECT.Enum(),
		Sections: []*waE2E.ListMessage_Section{
			{
				Title: proto.String("Menu"),
				Rows:  rows,
			},
		},
	}

	if quoted != nil {
		listMsg.ContextInfo = &waE2E.ContextInfo{
			StanzaID:      &quoted.Info.ID,
			Participant:   proto.String(quoted.Info.Sender.String()),
			QuotedMessage: quoted.Message,
		}
	}

	return s.Client.SendMessage(s.Ctx, jid, &waE2E.Message{
		ListMessage: listMsg,
	})
}

func (s *Socket) SendListMessage(jid types.JID, title string, description string, sections []ListSection, footer string, quoted *events.Message) (whatsmeow.SendResponse, error) {

	type nfRow struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description,omitempty"`
	}
	type nfSection struct {
		Title string  `json:"title"`
		Rows  []nfRow `json:"rows"`
	}

	var jsonSections []nfSection
	for _, sec := range sections {
		var jRows []nfRow
		for _, r := range sec.Rows {
			jRows = append(jRows, nfRow{
				ID:          r.ID,
				Title:       r.Title,
				Description: r.Description,
			})
		}
		jsonSections = append(jsonSections, nfSection{
			Title: sec.Title,
			Rows:  jRows,
		})
	}

	jsonBytes, _ := json.Marshal(jsonSections)
	btnConfig := SingleSelectButton(title, string(jsonBytes))

	var waButtons []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton
	waButtons = append(waButtons, &waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
		Name:             proto.String(btnConfig.Name),
		ButtonParamsJSON: proto.String(btnConfig.ButtonParamsJson),
	})

	interactiveMsg := &waE2E.InteractiveMessage{
		Header: &waE2E.InteractiveMessage_Header{
			Title: proto.String(title),
		},
		Body: &waE2E.InteractiveMessage_Body{
			Text: proto.String(description),
		},
		InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{
			NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
				Buttons:        waButtons,
				MessageVersion: proto.Int32(1),
			},
		},
	}

	if footer != "" {
		interactiveMsg.Footer = &waE2E.InteractiveMessage_Footer{
			Text: proto.String(footer),
		}
	}

	if quoted != nil {
		interactiveMsg.ContextInfo = &waE2E.ContextInfo{
			StanzaID:      &quoted.Info.ID,
			Participant:   proto.String(quoted.Info.Sender.String()),
			QuotedMessage: quoted.Message,
		}
	}

	bypassNodes := GetLumiBypassNodes()
	return s.Client.SendMessage(s.Ctx, jid, &waE2E.Message{
		ViewOnceMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				InteractiveMessage: interactiveMsg,
			},
		},
	}, whatsmeow.SendRequestExtra{AdditionalNodes: &bypassNodes})
}

func (s *Socket) SendButtonWithImage(jid types.JID, body string, buttons []string, footer string, imageURL string, quoted *events.Message) (whatsmeow.SendResponse, error) {
	if imageURL == "" {
		return s.SendButtonMessage(jid, body, buttons, footer, quoted)
	}

	resp, err := http.Get(imageURL)
	if err != nil {
		return s.SendButtonMessage(jid, body, buttons, footer, quoted)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return s.SendButtonMessage(jid, body, buttons, footer, quoted)
	}

	uploaded, err := s.Client.Upload(s.Ctx, data, whatsmeow.MediaImage)
	if err != nil {
		return s.SendButtonMessage(jid, body, buttons, footer, quoted)
	}

	isBiz := s.IsBusiness(jid)

	if isBiz {
		return s.sendBusinessButtonsWithImage(jid, body, buttons, footer, uploaded, quoted)
	}

	return s.sendPersonalButtonsWithImage(jid, body, buttons, footer, uploaded, quoted)
}

func (s *Socket) sendBusinessButtonsWithImage(jid types.JID, body string, buttons []string, footer string, uploaded whatsmeow.UploadResponse, quoted *events.Message) (whatsmeow.SendResponse, error) {
	var waButtons []*waE2E.ButtonsMessage_Button
	for i, b := range buttons {
		waButtons = append(waButtons, &waE2E.ButtonsMessage_Button{
			ButtonID: proto.String(fmt.Sprintf("id-%d", i)),
			ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
				DisplayText: proto.String(b),
			},
			Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
		})
	}

	msg := &waE2E.Message{
		ButtonsMessage: &waE2E.ButtonsMessage{
			ContentText: proto.String(body),
			FooterText:  proto.String(footer),
			HeaderType:  waE2E.ButtonsMessage_IMAGE.Enum(),
			Header: &waE2E.ButtonsMessage_ImageMessage{
				ImageMessage: &waE2E.ImageMessage{
					URL:           proto.String(uploaded.URL),
					DirectPath:    proto.String(uploaded.DirectPath),
					MediaKey:      uploaded.MediaKey,
					Mimetype:      proto.String("image/jpeg"),
					FileSHA256:    uploaded.FileSHA256,
					FileEncSHA256: uploaded.FileEncSHA256,
					FileLength:    proto.Uint64(uploaded.FileLength),
				},
			},
			Buttons: waButtons,
		},
	}

	if quoted != nil {
		msg.ButtonsMessage.ContextInfo = &waE2E.ContextInfo{
			StanzaID:      &quoted.Info.ID,
			Participant:   proto.String(quoted.Info.Sender.String()),
			QuotedMessage: quoted.Message,
		}
	}

	return s.Client.SendMessage(s.Ctx, jid, msg)
}

func (s *Socket) sendPersonalButtonsWithImage(jid types.JID, body string, buttons []string, footer string, uploaded whatsmeow.UploadResponse, quoted *events.Message) (whatsmeow.SendResponse, error) {
	var waButtons []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton
	for i, b := range buttons {
		waButtons = append(waButtons, &waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
			Name:             proto.String("quick_reply"),
			ButtonParamsJSON: proto.String(fmt.Sprintf(`{"display_text":"%s","id":"id-%d"}`, b, i)),
		})
	}

	interactiveMsg := &waE2E.InteractiveMessage{
		Header: &waE2E.InteractiveMessage_Header{
			Title:              proto.String(body),
			HasMediaAttachment: proto.Bool(true),
			Media: &waE2E.InteractiveMessage_Header_ImageMessage{
				ImageMessage: &waE2E.ImageMessage{
					URL:           proto.String(uploaded.URL),
					DirectPath:    proto.String(uploaded.DirectPath),
					MediaKey:      uploaded.MediaKey,
					Mimetype:      proto.String("image/jpeg"),
					FileSHA256:    uploaded.FileSHA256,
					FileEncSHA256: uploaded.FileEncSHA256,
					FileLength:    proto.Uint64(uploaded.FileLength),
				},
			},
		},
		Body: &waE2E.InteractiveMessage_Body{
			Text: proto.String(body),
		},
		InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{
			NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
				Buttons:        waButtons,
				MessageVersion: proto.Int32(1),
			},
		},
	}

	if footer != "" {
		interactiveMsg.Footer = &waE2E.InteractiveMessage_Footer{
			Text: proto.String(footer),
		}
	}

	if quoted != nil {
		interactiveMsg.ContextInfo = &waE2E.ContextInfo{
			StanzaID:      &quoted.Info.ID,
			Participant:   proto.String(quoted.Info.Sender.String()),
			QuotedMessage: quoted.Message,
		}
	}

	bypassNodes := GetLumiBypassNodes()
	return s.Client.SendMessage(s.Ctx, jid, &waE2E.Message{
		InteractiveMessage: interactiveMsg,
	}, whatsmeow.SendRequestExtra{AdditionalNodes: &bypassNodes})
}

func (s *Socket) SendQuickReplyButton(jid types.JID, body string, buttons []*ButtonConfig, footer string, quoted *events.Message) (whatsmeow.SendResponse, error) {
	var waButtons []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton
	for _, btn := range buttons {
		waButtons = append(waButtons, &waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
			Name:             proto.String(btn.Name),
			ButtonParamsJSON: proto.String(btn.ButtonParamsJson),
		})
	}

	interactiveMsg := &waE2E.InteractiveMessage{
		Body: &waE2E.InteractiveMessage_Body{
			Text: proto.String(body),
		},
		InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{
			NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
				Buttons:        waButtons,
				MessageVersion: proto.Int32(1),
			},
		},
	}

	if footer != "" {
		interactiveMsg.Footer = &waE2E.InteractiveMessage_Footer{
			Text: proto.String(footer),
		}
	}

	if quoted != nil {
		interactiveMsg.ContextInfo = &waE2E.ContextInfo{
			StanzaID:      &quoted.Info.ID,
			Participant:   proto.String(quoted.Info.Sender.String()),
			QuotedMessage: quoted.Message,
		}
	}

	bypassNodes := GetLumiBypassNodes()
	return s.Client.SendMessage(s.Ctx, jid, &waE2E.Message{
		InteractiveMessage: interactiveMsg,
	}, whatsmeow.SendRequestExtra{AdditionalNodes: &bypassNodes})
}

func (s *Socket) SendURLButton(jid types.JID, body string, buttons []*ButtonConfig, footer string, quoted *events.Message) (whatsmeow.SendResponse, error) {
	return s.SendQuickReplyButton(jid, body, buttons, footer, quoted)
}

func (s *Socket) SendCopyButton(jid types.JID, body string, buttons []*ButtonConfig, footer string, quoted *events.Message) (whatsmeow.SendResponse, error) {
	return s.SendQuickReplyButton(jid, body, buttons, footer, quoted)
}

func (s *Socket) SendCallButton(jid types.JID, body string, buttons []*ButtonConfig, footer string, quoted *events.Message) (whatsmeow.SendResponse, error) {
	return s.SendQuickReplyButton(jid, body, buttons, footer, quoted)
}

func (s *Socket) SendQuickReplyWithImage(jid types.JID, header *InteractiveHeader, body string, buttons []*ButtonConfig, footer string, quoted *events.Message) (whatsmeow.SendResponse, error) {
	var waButtons []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton
	for _, btn := range buttons {
		waButtons = append(waButtons, &waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
			Name:             proto.String(btn.Name),
			ButtonParamsJSON: proto.String(btn.ButtonParamsJson),
		})
	}

	interactiveMsg := &waE2E.InteractiveMessage{
		Header: BuildInteractiveHeader(header),
		Body: &waE2E.InteractiveMessage_Body{
			Text: proto.String(body),
		},
		InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{
			NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
				Buttons:        waButtons,
				MessageVersion: proto.Int32(1),
			},
		},
	}

	if footer != "" {
		interactiveMsg.Footer = &waE2E.InteractiveMessage_Footer{
			Text: proto.String(footer),
		}
	}

	if quoted != nil {
		interactiveMsg.ContextInfo = &waE2E.ContextInfo{
			StanzaID:      &quoted.Info.ID,
			Participant:   proto.String(quoted.Info.Sender.String()),
			QuotedMessage: quoted.Message,
		}
	}

	bypassNodes := GetLumiBypassNodes()
	return s.Client.SendMessage(s.Ctx, jid, &waE2E.Message{
		InteractiveMessage: interactiveMsg,
	}, whatsmeow.SendRequestExtra{AdditionalNodes: &bypassNodes})
}

func (s *Socket) SendNativeFlowButtonBusiness(jid types.JID, body string, buttons []*ButtonConfig, footer string, headerImage *waE2E.ImageMessage, quoted *events.Message) (whatsmeow.SendResponse, error) {
	var waButtons []*waE2E.ButtonsMessage_Button
	for _, btn := range buttons {
		waButtons = append(waButtons, &waE2E.ButtonsMessage_Button{
			ButtonID: proto.String(btn.Name),
			ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
				DisplayText: proto.String(btn.Name),
			},
			Type: waE2E.ButtonsMessage_Button_NATIVE_FLOW.Enum(),
			NativeFlowInfo: &waE2E.ButtonsMessage_Button_NativeFlowInfo{
				Name:       proto.String(btn.Name),
				ParamsJSON: proto.String(btn.ButtonParamsJson),
			},
		})
	}

	msg := &waE2E.Message{
		ButtonsMessage: &waE2E.ButtonsMessage{
			ContentText: proto.String(body),
			FooterText:  proto.String(footer),
			Buttons:     waButtons,
		},
	}

	if headerImage != nil {
		msg.ButtonsMessage.HeaderType = waE2E.ButtonsMessage_IMAGE.Enum()
		msg.ButtonsMessage.Header = &waE2E.ButtonsMessage_ImageMessage{
			ImageMessage: headerImage,
		}
	} else {
		msg.ButtonsMessage.HeaderType = waE2E.ButtonsMessage_EMPTY.Enum()
	}

	if quoted != nil {
		msg.ButtonsMessage.ContextInfo = &waE2E.ContextInfo{
			StanzaID:      &quoted.Info.ID,
			Participant:   proto.String(quoted.Info.Sender.String()),
			QuotedMessage: quoted.Message,
		}
	}

	return s.Client.SendMessage(s.Ctx, jid, msg)
}

func (s *Socket) SendCarousel(jid types.JID, body string, cards []CarouselCard, quoted *events.Message) (whatsmeow.SendResponse, error) {
	interactiveCards := make([]*waE2E.InteractiveMessage, 0, len(cards))

	for _, card := range cards {
		nativeButtons := make([]*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton, 0, len(card.Buttons))
		for _, btn := range card.Buttons {
			nativeButtons = append(nativeButtons, &waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
				Name:             proto.String(btn.Name),
				ButtonParamsJSON: proto.String(btn.ButtonParamsJson),
			})
		}

		im := &waE2E.InteractiveMessage{
			Header: BuildInteractiveHeader(card.Header),
			Body: &waE2E.InteractiveMessage_Body{
				Text: proto.String(card.Body),
			},
		}

		if card.Footer != "" {
			im.Footer = &waE2E.InteractiveMessage_Footer{
				Text: proto.String(card.Footer),
			}
		}

		if len(nativeButtons) > 0 {
			im.InteractiveMessage = &waE2E.InteractiveMessage_NativeFlowMessage_{
				NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
					Buttons:        nativeButtons,
					MessageVersion: proto.Int32(1),
				},
			}
		}

		interactiveCards = append(interactiveCards, im)
	}

	cardType := waE2E.InteractiveMessage_CarouselMessage_HSCROLL_CARDS.Enum()

	interactiveMsg := &waE2E.InteractiveMessage{
		Body: &waE2E.InteractiveMessage_Body{
			Text: proto.String(body),
		},
		InteractiveMessage: &waE2E.InteractiveMessage_CarouselMessage_{
			CarouselMessage: &waE2E.InteractiveMessage_CarouselMessage{
				Cards:            interactiveCards,
				MessageVersion:   proto.Int32(1),
				CarouselCardType: cardType,
			},
		},
	}

	if quoted != nil {
		interactiveMsg.ContextInfo = &waE2E.ContextInfo{
			StanzaID:      &quoted.Info.ID,
			Participant:   proto.String(quoted.Info.Sender.String()),
			QuotedMessage: quoted.Message,
		}
	}

	bypassNodes := GetLumiBypassNodes()
	return s.Client.SendMessage(s.Ctx, jid, &waE2E.Message{
		InteractiveMessage: interactiveMsg,
	}, whatsmeow.SendRequestExtra{AdditionalNodes: &bypassNodes})
}

func (s *Socket) SendCarouselWithImages(jid types.JID, body string, imageURLs []string, cardBodies []string, cardButtons [][]*ButtonConfig, quoted *events.Message) (whatsmeow.SendResponse, error) {
	if len(imageURLs) != len(cardBodies) {
		return whatsmeow.SendResponse{}, fmt.Errorf("jumlah imageURLs dan cardBodies harus sama")
	}

	var cards []CarouselCard
	for i := range imageURLs {
		img, err := s.uploadImage(imageURLs[i])
		if err != nil {
			continue
		}

		buttons := cardButtons[i]
		if buttons == nil {
			buttons = []*ButtonConfig{}
		}

		cards = append(cards, CarouselCard{
			Header: &InteractiveHeader{
				Title: "",
				Image: img,
			},
			Body:    cardBodies[i],
			Footer:  "",
			Buttons: buttons,
		})
	}

	return s.SendCarousel(jid, body, cards, quoted)
}

func (s *Socket) SendCarouselWithURLButtons(jid types.JID, body string, cards []CarouselCard, quoted *events.Message) (whatsmeow.SendResponse, error) {
	return s.SendCarousel(jid, body, cards, quoted)
}

func (s *Socket) SendCarouselViewOnce(jid types.JID, body string, cards []CarouselCard, quoted *events.Message) (whatsmeow.SendResponse, error) {
	interactiveCards := make([]*waE2E.InteractiveMessage, 0, len(cards))

	for _, card := range cards {
		nativeButtons := make([]*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton, 0, len(card.Buttons))
		for _, btn := range card.Buttons {
			nativeButtons = append(nativeButtons, &waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
				Name:             proto.String(btn.Name),
				ButtonParamsJSON: proto.String(btn.ButtonParamsJson),
			})
		}

		im := &waE2E.InteractiveMessage{
			Header: BuildInteractiveHeader(card.Header),
			Body: &waE2E.InteractiveMessage_Body{
				Text: proto.String(card.Body),
			},
		}

		if card.Footer != "" {
			im.Footer = &waE2E.InteractiveMessage_Footer{
				Text: proto.String(card.Footer),
			}
		}

		if len(nativeButtons) > 0 {
			im.InteractiveMessage = &waE2E.InteractiveMessage_NativeFlowMessage_{
				NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
					Buttons:        nativeButtons,
					MessageVersion: proto.Int32(1),
				},
			}
		}

		interactiveCards = append(interactiveCards, im)
	}

	cardType := waE2E.InteractiveMessage_CarouselMessage_HSCROLL_CARDS.Enum()

	interactiveMsg := &waE2E.InteractiveMessage{
		Body: &waE2E.InteractiveMessage_Body{
			Text: proto.String(body),
		},
		InteractiveMessage: &waE2E.InteractiveMessage_CarouselMessage_{
			CarouselMessage: &waE2E.InteractiveMessage_CarouselMessage{
				Cards:            interactiveCards,
				MessageVersion:   proto.Int32(1),
				CarouselCardType: cardType,
			},
		},
	}

	if quoted != nil {
		interactiveMsg.ContextInfo = &waE2E.ContextInfo{
			StanzaID:      &quoted.Info.ID,
			Participant:   proto.String(quoted.Info.Sender.String()),
			QuotedMessage: quoted.Message,
		}
	}

	bypassNodes := GetLumiBypassNodes()
	return s.Client.SendMessage(s.Ctx, jid, &waE2E.Message{
		InteractiveMessage: interactiveMsg,
	}, whatsmeow.SendRequestExtra{AdditionalNodes: &bypassNodes})
}

func (s *Socket) SendCollectionMessage(jid types.JID, body string, bizJID string, quoted *events.Message) (whatsmeow.SendResponse, error) {
	interactiveMsg := &waE2E.InteractiveMessage{
		Body: &waE2E.InteractiveMessage_Body{
			Text: proto.String(body),
		},
		InteractiveMessage: &waE2E.InteractiveMessage_CollectionMessage_{
			CollectionMessage: &waE2E.InteractiveMessage_CollectionMessage{
				BizJID:         proto.String(bizJID),
				MessageVersion: proto.Int32(1),
			},
		},
	}

	if quoted != nil {
		interactiveMsg.ContextInfo = &waE2E.ContextInfo{
			StanzaID:      &quoted.Info.ID,
			Participant:   proto.String(quoted.Info.Sender.String()),
			QuotedMessage: quoted.Message,
		}
	}

	bypassNodes := GetLumiBypassNodes()
	return s.Client.SendMessage(s.Ctx, jid, &waE2E.Message{
		InteractiveMessage: interactiveMsg,
	}, whatsmeow.SendRequestExtra{AdditionalNodes: &bypassNodes})
}

func (s *Socket) SendAdReply(jid types.JID, text string, title string, body string, thumbnailURL string, sourceURL string, quoted *events.Message) (whatsmeow.SendResponse, error) {
	var thumbnailBytes []byte
	if thumbnailURL != "" {
		resp, err := http.Get(thumbnailURL)
		if err == nil {
			defer resp.Body.Close()
			thumbnailBytes, _ = io.ReadAll(resp.Body)
		}
	}

	ctxInfo := &waE2E.ContextInfo{
		ExternalAdReply: &waE2E.ContextInfo_ExternalAdReplyInfo{
			Title:                 proto.String(title),
			Body:                  proto.String(body),
			ThumbnailURL:          proto.String(thumbnailURL),
			SourceURL:             proto.String(sourceURL),
			MediaType:             waE2E.ContextInfo_ExternalAdReplyInfo_IMAGE.Enum(),
			RenderLargerThumbnail: proto.Bool(true),
		},
	}

	if len(thumbnailBytes) > 0 {
		ctxInfo.ExternalAdReply.Thumbnail = thumbnailBytes
	}

	if quoted != nil {
		ctxInfo.StanzaID = &quoted.Info.ID
		ctxInfo.Participant = proto.String(quoted.Info.Sender.String())
		ctxInfo.QuotedMessage = quoted.Message
	}

	extendedMsg := &waE2E.ExtendedTextMessage{
		Text:        proto.String(text),
		ContextInfo: ctxInfo,
	}

	bypassNodes := GetLumiBypassNodes()
	return s.Client.SendMessage(s.Ctx, jid, &waE2E.Message{
		ExtendedTextMessage: extendedMsg,
	}, whatsmeow.SendRequestExtra{AdditionalNodes: &bypassNodes})
}

func GetNDLabsContextInfo(baseCtx *waE2E.ContextInfo) *waE2E.ContextInfo {
	if baseCtx == nil {
		baseCtx = &waE2E.ContextInfo{}
	}

	title := os.Getenv("BOT_NAME")
	if title == "" {
		title = "Powered by WhatsApp Bot"
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

	baseCtx.ExternalAdReply = &waE2E.ContextInfo_ExternalAdReplyInfo{
		Title:                 proto.String(title),
		Body:                  proto.String("WhatsApp Bot Assistant"),
		ThumbnailURL:          proto.String(thumbnail),
		SourceURL:             proto.String(sourceURL),
		MediaType:             waE2E.ContextInfo_ExternalAdReplyInfo_IMAGE.Enum(),
		RenderLargerThumbnail: proto.Bool(true),
	}
	return baseCtx
}

func (s *Socket) SendMedia(jid types.JID, url string, mediaType string, caption string, quoted *events.Message) (whatsmeow.SendResponse, error) {
	resp, err := http.Get(url)
	if err != nil {
		return whatsmeow.SendResponse{}, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return whatsmeow.SendResponse{}, err
	}

	return s.SendMediaBytes(jid, data, mediaType, caption, quoted)
}

func (s *Socket) SendMediaBytes(jid types.JID, data []byte, mediaType string, caption string, quoted *events.Message) (whatsmeow.SendResponse, error) {

	var waMediaType whatsmeow.MediaType
	var mimetype string
	switch strings.ToLower(mediaType) {
	case "image":
		waMediaType = whatsmeow.MediaImage
		mimetype = "image/jpeg"
	case "video":
		waMediaType = whatsmeow.MediaVideo
		mimetype = "video/mp4"
	case "audio":
		waMediaType = whatsmeow.MediaAudio
		mimetype = "audio/mpeg"
	case "document":
		waMediaType = whatsmeow.MediaDocument
		mimetype = "video/mp4"
	case "ptt", "vn":
		waMediaType = whatsmeow.MediaAudio
		mimetype = "audio/ogg; codecs=opus"
	case "sticker":
		waMediaType = whatsmeow.MediaImage
		mimetype = "image/webp"
	default:
		waMediaType = whatsmeow.MediaImage
		mimetype = "image/jpeg"
	}

	uploaded, err := s.Client.Upload(s.Ctx, data, waMediaType)
	if err != nil {
		return whatsmeow.SendResponse{}, err
	}

	var ctxInfo *waE2E.ContextInfo
	if quoted != nil {
		ctxInfo = &waE2E.ContextInfo{
			StanzaID:      &quoted.Info.ID,
			Participant:   proto.String(quoted.Info.Sender.String()),
			QuotedMessage: quoted.Message,
		}
	}

	mediaTypeLower := strings.ToLower(mediaType)
	if mediaTypeLower != "ptt" && mediaTypeLower != "vn" {
		ctxInfo = GetNDLabsContextInfo(ctxInfo)
	}

	msg := &waE2E.Message{}

	switch mediaTypeLower {
	case "sticker":
		msg.StickerMessage = &waE2E.StickerMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(mimetype),
			FileSHA256:    uploaded.FileSHA256,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileLength:    proto.Uint64(uploaded.FileLength),
			IsAnimated:    proto.Bool(isAnimatedWebP(data)),
			Width:         proto.Uint32(512),
			Height:        proto.Uint32(512),
			ContextInfo:   ctxInfo,
		}
	case "image":
		msg.ImageMessage = &waE2E.ImageMessage{
			Caption:       proto.String(caption),
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(mimetype),
			FileSHA256:    uploaded.FileSHA256,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileLength:    proto.Uint64(uploaded.FileLength),
			ContextInfo:   ctxInfo,
		}
	case "video":
		msg.VideoMessage = &waE2E.VideoMessage{
			Caption:       proto.String(caption),
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(mimetype),
			FileSHA256:    uploaded.FileSHA256,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileLength:    proto.Uint64(uploaded.FileLength),
			ContextInfo:   ctxInfo,
		}
	case "audio":
		msg.AudioMessage = &waE2E.AudioMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(mimetype),
			FileSHA256:    uploaded.FileSHA256,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileLength:    proto.Uint64(uploaded.FileLength),
			ContextInfo:   ctxInfo,
		}
	case "ptt", "vn":
		if ctxInfo == nil {
			ctxInfo = &waE2E.ContextInfo{}
		}

		ctxInfo.IsForwarded = proto.Bool(false)
		ctxInfo.ForwardingScore = proto.Uint32(0)

		msg.AudioMessage = &waE2E.AudioMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(mimetype),
			FileSHA256:    uploaded.FileSHA256,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileLength:    proto.Uint64(uploaded.FileLength),
			Seconds:       proto.Uint32(2),
			Waveform:      []byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x34, 0x4d, 0x50, 0x4f, 0x42, 0x3b, 0x45, 0x4e, 0x51, 0x54, 0x54, 0x53, 0x52, 0x50, 0x52, 0x54, 0x53, 0x52, 0x4f, 0x4d, 0x42, 0x35, 0x30, 0x2d, 0x20, 0xc, 0x2, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			ContextInfo:   ctxInfo,
			PTT:           proto.Bool(true),
		}
	case "document":

		fileName := "video.mp4"
		if caption != "" {
			fileName = caption + ".mp4"
			if len(fileName) > 60 {
				fileName = fileName[:60] + ".mp4"
			}
		}
		msg.DocumentMessage = &waE2E.DocumentMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(mimetype),
			FileSHA256:    uploaded.FileSHA256,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileLength:    proto.Uint64(uploaded.FileLength),
			Title:         proto.String(caption),
			FileName:      proto.String(fileName),
			ContextInfo:   ctxInfo,
		}
	}

	return s.Client.SendMessage(s.Ctx, jid, msg)
}

func (s *Socket) SendDocumentBytes(jid types.JID, data []byte, fileName, mimetype, caption string, quoted *events.Message) (whatsmeow.SendResponse, error) {
	if mimetype == "" {
		mimetype = "text/plain"
	}
	if fileName == "" {
		fileName = "file.txt"
	}

	uploaded, err := s.Client.Upload(s.Ctx, data, whatsmeow.MediaDocument)
	if err != nil {
		return whatsmeow.SendResponse{}, err
	}

	var ctxInfo *waE2E.ContextInfo
	if quoted != nil {
		ctxInfo = &waE2E.ContextInfo{
			StanzaID:      &quoted.Info.ID,
			Participant:   proto.String(quoted.Info.Sender.String()),
			QuotedMessage: quoted.Message,
		}
	}

	msg := &waE2E.Message{
		DocumentMessage: &waE2E.DocumentMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(mimetype),
			FileSHA256:    uploaded.FileSHA256,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileLength:    proto.Uint64(uploaded.FileLength),
			Title:         proto.String(fileName),
			FileName:      proto.String(fileName),
			Caption:       proto.String(caption),
			ContextInfo:   ctxInfo,
		},
	}

	return s.Client.SendMessage(s.Ctx, jid, msg)
}

func (s *Socket) SendContact(jid types.JID, name, phoneNumber string, quoted *events.Message) (whatsmeow.SendResponse, error) {
	vcard := fmt.Sprintf("BEGIN:VCARD\nVERSION:3.0\nFN:%s\nTEL;type=CELL;type=VOICE;waid=%s:+%s\nEND:VCARD", name, phoneNumber, phoneNumber)

	msg := &waE2E.Message{
		ContactMessage: &waE2E.ContactMessage{
			DisplayName: proto.String(name),
			Vcard:       proto.String(vcard),
		},
	}

	if quoted != nil {
		msg.ContactMessage.ContextInfo = &waE2E.ContextInfo{
			StanzaID:      &quoted.Info.ID,
			Participant:   proto.String(quoted.Info.Sender.String()),
			QuotedMessage: quoted.Message,
		}
	}

	return s.Client.SendMessage(s.Ctx, jid, msg)
}

func (s *Socket) SendPoll(jid types.JID, question string, options []string, selectableCount int) (whatsmeow.SendResponse, error) {
	return s.Client.SendMessage(s.Ctx, jid, s.Client.BuildPollCreation(question, options, selectableCount))
}

func (s *Socket) SendSticker(jid types.JID, data []byte, quoted *events.Message) (whatsmeow.SendResponse, error) {
	animated := isAnimatedWebP(data)

	uploaded, err := s.Client.Upload(s.Ctx, data, whatsmeow.MediaImage)
	if err != nil {
		return whatsmeow.SendResponse{}, err
	}

	stickerMsg := &waE2E.StickerMessage{
		URL:           proto.String(uploaded.URL),
		DirectPath:    proto.String(uploaded.DirectPath),
		MediaKey:      uploaded.MediaKey,
		Mimetype:      proto.String("image/webp"),
		FileSHA256:    uploaded.FileSHA256,
		FileEncSHA256: uploaded.FileEncSHA256,
		FileLength:    proto.Uint64(uploaded.FileLength),
		IsAnimated:    proto.Bool(animated),
		Width:         proto.Uint32(512),
		Height:        proto.Uint32(512),
	}

	if quoted != nil {
		stickerMsg.ContextInfo = &waE2E.ContextInfo{
			StanzaID:      &quoted.Info.ID,
			Participant:   proto.String(quoted.Info.Sender.String()),
			QuotedMessage: quoted.Message,
		}
	}

	return s.Client.SendMessage(s.Ctx, jid, &waE2E.Message{
		StickerMessage: stickerMsg,
	})
}

func (s *Socket) SendImageBytes(jid types.JID, data []byte, caption string, quoted *events.Message) (whatsmeow.SendResponse, error) {
	uploaded, err := s.Client.Upload(s.Ctx, data, whatsmeow.MediaImage)
	if err != nil {
		return whatsmeow.SendResponse{}, err
	}

	msg := &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String("image/jpeg"),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
			Caption:       proto.String(caption),
		},
	}
	if quoted != nil {
		msg.ImageMessage.ContextInfo = &waE2E.ContextInfo{
			StanzaID:      proto.String(quoted.Info.ID),
			Participant:   proto.String(quoted.Info.Sender.String()),
			QuotedMessage: quoted.Message,
		}
	}
	return s.Client.SendMessage(s.Ctx, jid, msg)
}

func (m *WAMessage) SenderPN() string {
	jid := m.Sender
	if jid.Server == "lid" || jid.Server == types.HiddenUserServer {
		if pn, err := m.Socket.Client.Store.LIDs.GetPNForLID(m.Ctx, jid); err == nil && !pn.IsEmpty() {
			jid = pn
		}
	}
	return jid.String()
}

func (m *WAMessage) SendAIRichMessage(builder *AIRichBuilder) (whatsmeow.SendResponse, error) {
	msg := builder.Build()
	return m.RelayMessage(msg, "bypass")
}

func (m *WAMessage) SendHydratedTemplate(text, footer string, buttons []*waE2E.HydratedTemplateButton) (whatsmeow.SendResponse, error) {
	template := &waE2E.TemplateMessage_HydratedFourRowTemplate{
		HydratedContentText: proto.String(text),
		HydratedFooterText:  proto.String(footer),
		HydratedButtons:     buttons,
	}

	msg := &waE2E.Message{
		TemplateMessage: &waE2E.TemplateMessage{
			HydratedTemplate: template,
			Format: &waE2E.TemplateMessage_HydratedFourRowTemplate_{
				HydratedFourRowTemplate: template,
			},
		},
	}

	bypassNodes := []waBinary.Node{
		{
			Tag:   "hsm",
			Attrs: waBinary.Attrs{"category": "MARKETING"},
		},
	}

	return m.RelayMessage(msg, whatsmeow.SendRequestExtra{AdditionalNodes: &bypassNodes})
}

func (s *Socket) SendButtonWithLocation(jid types.JID, title, subtitle, body string, buttons []string, footer string, thumbnail []byte, quoted *events.Message) (whatsmeow.SendResponse, error) {
	var waButtons []*waE2E.ButtonsMessage_Button
	for _, btn := range buttons {
		btnID := "btn_" + sanitizeJSON(btn)
		waButtons = append(waButtons, &waE2E.ButtonsMessage_Button{
			ButtonID: proto.String(btnID),
			ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
				DisplayText: proto.String(btn),
			},
			Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
		})
	}

	btnMsg := &waE2E.ButtonsMessage{
		ContentText: proto.String(body),
		FooterText:  proto.String(footer),
		Buttons:     waButtons,
		HeaderType:  waE2E.ButtonsMessage_LOCATION.Enum(),
		Header: &waE2E.ButtonsMessage_LocationMessage{
			LocationMessage: &waE2E.LocationMessage{
				DegreesLatitude:  proto.Float64(0),
				DegreesLongitude: proto.Float64(0),
				Name:             proto.String(title),
				Address:          proto.String(subtitle),
				JPEGThumbnail:    thumbnail,
			},
		},
	}

	if quoted != nil {
		btnMsg.ContextInfo = &waE2E.ContextInfo{
			StanzaID:      &quoted.Info.ID,
			Participant:   proto.String(quoted.Info.Sender.String()),
			QuotedMessage: quoted.Message,
		}
	}

	msg := &waE2E.Message{
		ButtonsMessage: btnMsg,
	}

	bypassNodes := GetLumiBypassNodes()
	return s.Client.SendMessage(s.Ctx, jid, msg, whatsmeow.SendRequestExtra{AdditionalNodes: &bypassNodes})
}

func (s *Socket) SendVideoBytes(jid types.JID, data []byte, caption string, quoted *events.Message) (whatsmeow.SendResponse, error) {
	uploaded, err := s.Client.Upload(s.Ctx, data, whatsmeow.MediaVideo)
	if err != nil {
		return whatsmeow.SendResponse{}, err
	}

	msg := &waE2E.Message{
		VideoMessage: &waE2E.VideoMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String("video/mp4"),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
			Caption:       proto.String(caption),
		},
	}
	if quoted != nil {
		msg.VideoMessage.ContextInfo = &waE2E.ContextInfo{
			StanzaID:      proto.String(quoted.Info.ID),
			Participant:   proto.String(quoted.Info.Sender.String()),
			QuotedMessage: quoted.Message,
		}
	}
	return s.Client.SendMessage(s.Ctx, jid, msg)
}
