# ndlabs-bot

Base Bot WhatsApp yang cepat, ringan, dan mudah dikembangkan, ditulis menggunakan Go.
Ditenagai oleh [whatsmeow](https://github.com/tulir/whatsmeow).

## Fitur Utama

- **Ringan**: Penggunaan memori sangat minim dan dioptimalkan untuk kecepatan.
- **Sistem Plugin**: Sangat mudah dikembangkan dengan arsitektur plugin yang modular.
- **Tanpa Bloatware**: Hanya menyertakan fitur-fitur esensial (polosan). Base yang bersih untuk pengembangan bot kamu sendiri.
- **Metrik Real-time**: Pemantauan sistem yang akurat sudah terpasang otomatis (`.ping` menampilkan RAM & CPU Load yang sesungguhnya).

## Persyaratan (Requirements)

- Go 1.20 atau versi lebih baru
- SQLite (untuk menyimpan database)

## Cara Memulai (Quick Start)

1. **Clone repositori ini:**
   ```bash
   git clone https://github.com/Nahidabyte/ndlabs-bot.git
   cd ndlabs-bot
   ```

2. **Konfigurasi Environment:**
   Salin `.env.example` menjadi `.env.local` lalu sesuaikan variabel di dalamnya.
   ```bash
   cp .env.example .env.local
   ```

3. **Jalankan Bot:**
   ```bash
   go mod tidy
   go run main.go
   ```
   Atau jika ingin di-build (dicompile):
   ```bash
   go build -o botwa
   ./botwa
   ```

## Struktur Folder

- `bot/` - Inti sistem bot, koneksi & event handler (whatsmeow).
- `database/` - Tempat penyimpanan database SQLite dan data sesi koneksi WA (`.device`).
- `plugins/` - Tempat kamu menaruh script command/plugin buatanmu.
- `plugin/` - File interface dan Base class untuk menyusun sistem plugin.

## Extending / Menambah Plugin Baru

Base bot ini menyediakan cara yang super mudah untuk membuat plugin baru. Cukup buat file `.go` baru di dalam folder `plugins/` (contoh: `plugins/mycommand.go`), lalu ikuti struktur di bawah ini:

```go
package plugins

import (
    "wa-bot/plugin"
)

func init() {
    plugin.RegisterPlugin(plugin.Cmd{
        Name:     "halo",              // Nama command utama (contoh: .halo)
        Category: "general",           // Kategori di menu
        Alias:    []string{"hi", "helo"}, // Alias command lain
        Desc:     "Command sapaan",    // Deskripsi di menu
        Exec:     haloCommand,         // Fungsi yang dieksekusi
    })
}

func haloCommand(ctx *plugin.CommandContext) {
    // Membalas chat text biasa
    ctx.Msg.Reply("Halo juga! 👋")
    
    // Atau membalas dengan tag pesan pengguna (quoted)
    // ctx.Msg.ReplyQuoted("Halo juga dari bot!")
}
```

Itu saja! Command kamu otomatis akan terbaca oleh bot dan langsung masuk ke daftar menu. Untuk mengirim pesan interaktif (Rich Message), kamu bisa lihat referensi di bawah.

## Rich Message Examples (Interactive Messages)

This base bot supports various types of rich and interactive WhatsApp messages out of the box. You can call these directly from your plugins via the `ctx.Msg` object. 

### 1. Button Message (Basic)
```go
ctx.Msg.SendButtonMessage(
    "Contoh pesan tombol:",
    []string{"A", "B", "C"},
    "Test tombol footer",
)
```

### 2. Button with Image
```go
ctx.Msg.SendButtonWithImage(
    "Contoh tombol dengan gambar:",
    []string{"A", "B", "C"},
    "Test tombol+gambar",
    "https://images.pexels.com/photos/414612/pexels-photo-414612.jpeg",
)
```

### 3. Button with Location Wrapper (Bypass)
```go
// thumbnailBytes bisa diisi []byte gambar, atau nil jika tidak perlu
var thumbBytes []byte 
ctx.Msg.SendButtonWithLocation(
    "Judul Header",
    "Subjudul Header",
    "Ini adalah text body pesannya",
    []string{"Tombol 1", "Tombol 2"},
    "Teks footer",
    thumbBytes,
)
```

### 4. Quick Reply (ID-based Buttons)
```go
ctx.Msg.SendQuickReply(
    "Contoh quick reply:",
    []*lib.ButtonConfig{
        lib.QuickReplyButton("Pilih A", "A"),
        lib.QuickReplyButton("Pilih B", "B"),
    },
    "Footer quick reply",
)
```

### 5. Interactive Action Buttons (URL, Copy, Call)
```go
// URL Button
ctx.Msg.SendURLButtons("Buka link ini:", []*lib.ButtonConfig{
    lib.URLButton("Buka Website", "https://example.com"),
}, "Footer")

// Copy Button
ctx.Msg.SendCopyButtons("Salin text ini:", []*lib.ButtonConfig{
    lib.CopyButton("Salin Kode", "TEST123"),
}, "Footer")

// Call Button
ctx.Msg.SendCallButtons("Hubungi kami:", []*lib.ButtonConfig{
    lib.CallButton("Telepon", "+628123456789"),
}, "Footer")
```

### 6. Carousel Message
```go
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
```

### 7. List Message (Dropdown)
```go
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
```

### 8. Custom Thumbnail (AdReply)
```go
ctx.Msg.SendAdReply(
    "Contoh pesan dengan popup custom thumbnail.",
    "Judul AdReply",
    "Ini adalah body text",
    "https://images.pexels.com/photos/414612/pexels-photo-414612.jpeg", // Gambar
    "https://example.com", // Link tujuan
)
```

### 9. Polling
```go
ctx.Msg.SendPoll(
    "Pilih bahasa favorit kamu:",
    []string{"Golang", "Python", "Javascript"},
    1, // Max opsi yang bisa dipilih
)
```

### 10. Kirim Gambar / Media langsung
```go
ctx.Msg.SendMessage(plugin.MessageOptions{
    Text:      "Contoh kirim media image",
    MediaURL:  "https://images.pexels.com/photos/414612/pexels-photo-414612.jpeg",
    MediaType: "image",
    Caption:   "Contoh caption media",
})
```

### 11. AI Rich Message (Meta AI Format)
```go
builder := lib.NewAIRichBuilder()
builder.SetTitle("🚀 NIXCODE AI")
builder.AddText("Ini teks dengan dukungan *Markdown* penuh.\n\nContoh Link: [Google](https://google.com)\nAuto citation: [](https://openai.com)")

// Menambahkan Code Block
builder.AddCode("javascript", "console.log('Hello World');")

// Menambahkan Tabel
builder.AddTable([][]string{
    {"Nama", "Role"},
    {"Nixel", "Developer"},
})

// Menambahkan Sumber Referensi (Source citations)
builder.AddSource([][]string{
    {"https://google.com/favicon.ico", "https://google.com", "Google Search"},
})

// Menambahkan Multi-Image
builder.AddImage([]string{
    "https://example.com/image1.jpg",
    "https://example.com/image2.jpg",
})

// Kirim
ctx.Msg.SendAIRichMessage(builder)
```

## License

MIT License
