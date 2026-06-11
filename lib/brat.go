package lib

import (
	"bytes"
	"fmt"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font/gofont/goregular"
)

var defaultFont *truetype.Font

func init() {
	var err error
	fontPath := filepath.Join("assets", "arialnarrow.ttf")
	fontData, err := os.ReadFile(fontPath)
	if err == nil {
		defaultFont, err = truetype.Parse(fontData)
	}

	if err != nil || defaultFont == nil {
		fmt.Printf("⚠️ Arial Narrow font not found at %s, falling back to goregular\n", fontPath)
		defaultFont, err = truetype.Parse(goregular.TTF)
		if err != nil {
			panic(err)
		}
	}
}

func setFontSize(dc *gg.Context, size float64) {
	if defaultFont != nil {
		face := truetype.NewFace(defaultFont, &truetype.Options{Size: size})
		dc.SetFontFace(face)
	}
}

type BratConfig struct {
	W           int
	H           int
	BoxW        int
	BoxH        int
	BoxPad      int
	LineHeight  float64
	BaselineAdj float64
	FontName    string
	FontWeight  int
	FontSize    float64
	FSMin       int
	FSMax       int
	Blur        int
	ColorBG     string
	ColorBox    string
	ColorText   string
	DebugMode   bool
}

type BratTheme struct {
	ColorBG   string
	ColorBox  string
	ColorText string
}

var DefaultConfig = BratConfig{
	W:           500,
	H:           500,
	BoxW:        500,
	BoxH:        500,
	BoxPad:      20,
	LineHeight:  1.08,
	BaselineAdj: 0.75,
	FontName:    "Arial",
	FontWeight:  400,
	FSMin:       8,
	FSMax:       130,
	Blur:        2,
	ColorBG:     "#ffffff",
	ColorBox:    "#ffffff",
	ColorText:   "#000000",
}

var Themes = map[string]BratTheme{
	"white": {
		ColorBG:   "#ffffff",
		ColorBox:  "#ffffff",
		ColorText: "#000000",
	},
	"black": {
		ColorBG:   "#000000",
		ColorBox:  "#000000",
		ColorText: "#ffffff",
	},
	"brat": {
		ColorBG:   "#8ace00",
		ColorBox:  "#8ace00",
		ColorText: "#000000",
	},
	"neon": {
		ColorBG:   "#39ff14",
		ColorBox:  "#39ff14",
		ColorText: "#000000",
	},
	"crimson": {
		ColorBG:   "#dc143c",
		ColorBox:  "#dc143c",
		ColorText: "#ffffff",
	},
	"midnight": {
		ColorBG:   "#191970",
		ColorBox:  "#191970",
		ColorText: "#ffffff",
	},
	"lime": {
		ColorBG:   "#00ff00",
		ColorBox:  "#00ff00",
		ColorText: "#000000",
	},
	"pink": {
		ColorBG:   "#ff69b4",
		ColorBox:  "#ff69b4",
		ColorText: "#ffffff",
	},
}

func ParseHexColor(hex string) (color.Color, error) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return nil, fmt.Errorf("invalid hex color: %s", hex)
	}
	var r, g, b uint8
	_, err := fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b)
	if err != nil {
		return nil, err
	}
	return color.RGBA{R: r, G: g, B: b, A: 255}, nil
}

func BratGen(text string, theme string) ([]byte, error) {
	cfg := DefaultConfig

	if t, ok := Themes[theme]; ok {
		cfg.ColorBG = t.ColorBG
		cfg.ColorBox = t.ColorBox
		cfg.ColorText = t.ColorText
	}

	bgColor, err := ParseHexColor(cfg.ColorBG)
	if err != nil {
		return nil, err
	}
	boxColor, err := ParseHexColor(cfg.ColorBox)
	if err != nil {
		return nil, err
	}
	textColor, err := ParseHexColor(cfg.ColorText)
	if err != nil {
		return nil, err
	}

	dc := gg.NewContext(cfg.W, cfg.H)

	dc.SetColor(bgColor)
	dc.Clear()

	bx := float64(cfg.W-cfg.BoxW) / 2
	by := float64(cfg.H-cfg.BoxH) / 2
	dc.SetColor(boxColor)
	dc.DrawRectangle(bx, by, float64(cfg.BoxW), float64(cfg.BoxH))
	dc.Fill()

	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, "\n", " ")
	for strings.Contains(text, "  ") {
		text = strings.ReplaceAll(text, "  ", " ")
	}

	if text == "" {

		buf := new(bytes.Buffer)
		png.Encode(buf, dc.Image())
		return buf.Bytes(), nil
	}

	dc.SetColor(textColor)

	txW := float64(cfg.BoxW - cfg.BoxPad*2)
	txH := float64(cfg.BoxH - cfg.BoxPad*2)
	txX := bx + float64(cfg.BoxPad)
	txY := by + float64(cfg.BoxPad)

	words := strings.Split(text, " ")

	wrapText := func(fontSize float64) [][]string {
		setFontSize(dc, fontSize)
		spaceW, _ := dc.MeasureString(" ")
		var lines [][]string
		var curLine []string
		var curWidth float64

		for _, word := range words {
			ww, _ := dc.MeasureString(word)
			if len(curLine) == 0 {
				curLine = append(curLine, word)
				curWidth = ww
			} else if curWidth+spaceW+ww <= txW {
				curLine = append(curLine, word)
				curWidth += spaceW + ww
			} else {
				lines = append(lines, curLine)
				curLine = []string{word}
				curWidth = ww
			}
		}
		if len(curLine) > 0 {
			lines = append(lines, curLine)
		}
		return lines
	}

	lo := cfg.FSMin
	hi := cfg.FSMax
	bestFontSize := float64(lo)

	for lo <= hi {
		mid := (lo + hi) / 2
		fontSize := float64(mid)
		setFontSize(dc, fontSize)
		spaceW, _ := dc.MeasureString(" ")

		lines := wrapText(fontSize)
		totalH := float64(len(lines)) * fontSize * cfg.LineHeight

		maxLineW := 0.0
		for _, line := range lines {
			lineW := 0.0
			for i, word := range line {
				ww, _ := dc.MeasureString(word)
				lineW += ww
				if i > 0 {
					lineW += spaceW
				}
			}
			if lineW > maxLineW {
				maxLineW = lineW
			}
		}

		if maxLineW <= txW && totalH <= txH {
			bestFontSize = fontSize
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}

	fontSize := bestFontSize
	lines := wrapText(fontSize)
	lineHeight := fontSize * cfg.LineHeight

	setFontSize(dc, fontSize)
	y := txY + fontSize*cfg.BaselineAdj

	for _, line := range lines {
		if len(line) == 0 {
			y += lineHeight
			continue
		}

		if len(line) == 1 {
			dc.DrawString(line[0], txX, y)
			y += lineHeight
			continue
		}

		var totalW float64
		wordWidths := make([]float64, len(line))
		for i, w := range line {
			width, _ := dc.MeasureString(w)
			wordWidths[i] = width
			totalW += width
		}

		gap := (txW - totalW) / float64(len(line)-1)
		curX := txX

		for i, w := range line {
			dc.DrawString(w, curX, y)
			curX += wordWidths[i] + gap
		}

		y += lineHeight
	}

	buf := new(bytes.Buffer)
	err = png.Encode(buf, dc.Image())
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
