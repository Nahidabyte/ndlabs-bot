package plugins

import (
	"fmt"
	"strings"
	"wa-bot/lib"
	"wa-bot/plugin"

	"go.mau.fi/whatsmeow"
)

func init() {
	plugin.RegisterPlugin(plugin.Cmd{
		Name:     "jadibot",
		Category: "main",
		Desc:     "Menumpang koneksi bot (Clone Bot)",
		Exec:     jadibot,
	})

	plugin.RegisterPlugin(plugin.Cmd{
		Name:     "stopjadibot",
		Category: "main",
		Desc:     "Berhenti menjadi bot clone",
		Exec:     stopjadibot,
	})

	plugin.RegisterPlugin(plugin.Cmd{
		Name:     "listjadibot",
		Category: "owner",
		Desc:     "Melihat daftar bot clone yang aktif",
		Exec:     listjadibot,
	})
}

func jadibot(ctx *plugin.CommandContext) {
	senderJID := ctx.Msg.Sender.ToNonAD()
	if senderJID.Server == "lid" && !ctx.Msg.RawMessage.Info.SenderAlt.IsEmpty() {
		senderJID = ctx.Msg.RawMessage.Info.SenderAlt.ToNonAD()
	}
	senderJIDString := senderJID.String()

	mainBotJID := ""
	if ctx.Msg.Socket.Client.Store.ID != nil {
		mainBotJID = ctx.Msg.Socket.Client.Store.ID.ToNonAD().String()
	}

	if mainBotJID != "" && senderJIDString == mainBotJID {
		ctx.Msg.Reply("❌ Anda tidak bisa menjadi jadibot dengan nomor bot utama.")
		return
	}

	if lib.JManager.GetClient(senderJIDString) != nil {
		ctx.Msg.Reply("❌ Anda sudah memiliki sesi jadibot yang aktif. Gunakan !stopjadibot untuk berhenti.")
		return
	}

	client, err := lib.JManager.CreateJadibot(ctx.Ctx, senderJIDString)
	if err != nil {
		ctx.Msg.Reply("❌ Gagal inisialisasi jadibot: " + err.Error())
		return
	}

	if err := client.Connect(); err != nil {
		ctx.Msg.Reply("❌ Gagal menghubungkan ke WhatsApp: " + err.Error())
		return
	}

	phone := senderJID.User
	if phone == "" {
		ctx.Msg.Reply("❌ Tidak dapat mendeteksi nomor telepon Anda untuk pairing.")
		return
	}

	code, err := client.PairPhone(ctx.Ctx, phone, true, whatsmeow.PairClientChrome, "Chrome (Windows)")
	if err != nil {
		ctx.Msg.Reply("❌ Gagal mendapatkan pairing code: " + err.Error())
		return
	}

	res := fmt.Sprintf("🤖 *JADIBOT SESSION*\n\n")
	res += fmt.Sprintf("Nomor: %s\n", phone)
	res += fmt.Sprintf("Kode Pairing: *%s*\n\n", code)
	res += "_Silakan masukkan kode di atas pada menu 'Tautkan Perangkat' di WhatsApp Anda._"

	ctx.Msg.Reply(res)
}

func stopjadibot(ctx *plugin.CommandContext) {
	senderJID := ctx.Msg.Sender.ToNonAD()
	if senderJID.Server == "lid" && !ctx.Msg.RawMessage.Info.SenderAlt.IsEmpty() {
		senderJID = ctx.Msg.RawMessage.Info.SenderAlt.ToNonAD()
	}
	senderJIDString := senderJID.String()

	if lib.JManager.GetClient(senderJIDString) == nil {
		ctx.Msg.Reply("❌ Anda tidak memiliki sesi jadibot aktif.")
		return
	}

	lib.JManager.RemoveClient(senderJIDString)
	ctx.Msg.Reply("✅ Sesi jadibot telah dihentikan.")
}

func listjadibot(ctx *plugin.CommandContext) {
	lib.JManager.Mu.RLock()
	defer lib.JManager.Mu.RUnlock()

	if len(lib.JManager.Clients) == 0 {
		ctx.Msg.Reply("ℹ️ Tidak ada bot clone yang aktif saat ini.")
		return
	}

	var sb strings.Builder
	sb.WriteString("📋 *LIST JADIBOT AKTIF*\n\n")
	for jid := range lib.JManager.Clients {
		sb.WriteString(fmt.Sprintf("• %s\n", jid))
	}

	ctx.Msg.Reply(sb.String())
}
