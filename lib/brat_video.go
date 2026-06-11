package lib

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type BratVideoOptions struct {
	OutputFormat  string
	Theme         string
	BPM           float64
	FrameDuration float64
	LastFrameHold float64
	MaxWordLayer  int
	DebugMode     bool
}

func TokenizeText(text string) []string {
	if text == "" {
		return []string{}
	}

	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, ",", " ")
	text = strings.ReplaceAll(text, "，", " ")

	for strings.Contains(text, "  ") {
		text = strings.ReplaceAll(text, "  ", " ")
	}

	if text == "" {
		return []string{}
	}

	words := strings.Fields(text)
	var tokens []string

	for _, word := range words {
		var currentWord strings.Builder
		for _, r := range word {

			if (r >= 0x1F300 && r <= 0x1FAFF) || (r >= 0x2600 && r <= 0x27BF) {
				if currentWord.Len() > 0 {
					tokens = append(tokens, currentWord.String())
					currentWord.Reset()
				}
				tokens = append(tokens, string(r))
			} else {
				currentWord.WriteRune(r)
			}
		}
		if currentWord.Len() > 0 {
			tokens = append(tokens, currentWord.String())
		}
	}

	return tokens
}

func SplitIntoLayers(tokens []string, maxPerLayer int) [][]string {
	if maxPerLayer <= 0 || maxPerLayer >= len(tokens) {
		return [][]string{tokens}
	}

	var layers [][]string
	for i := 0; i < len(tokens); i += maxPerLayer {
		end := i + maxPerLayer
		if end > len(tokens) {
			end = len(tokens)
		}
		layers = append(layers, tokens[i:end])
	}
	return layers
}

type BratFrame struct {
	Text        string
	LayerIdx    int
	IsLastLayer bool
	Duration    float64
}

func BuildFrameSequence(tokens []string, maxPerLayer int) []BratFrame {
	layers := SplitIntoLayers(tokens, maxPerLayer)
	var frames []BratFrame

	for layerIdx, layer := range layers {
		layerText := ""
		for tokenIdx, token := range layer {
			if tokenIdx > 0 {
				layerText += " "
			}
			layerText += token

			isLastInLayer := tokenIdx == len(layer)-1
			frames = append(frames, BratFrame{
				Text:        layerText,
				LayerIdx:    layerIdx,
				IsLastLayer: isLastInLayer,
			})
		}
	}

	return frames
}

func ResolveDurations(frames []BratFrame, opts BratVideoOptions) []BratFrame {
	frameDur := opts.FrameDuration
	if frameDur <= 0 {
		frameDur = 0.7
	}

	lastFrameDur := opts.LastFrameHold
	if lastFrameDur <= 0 {
		lastFrameDur = 1.5
	}

	if opts.BPM > 0 {
		frameDur = 60.0 / opts.BPM
	}

	for i := range frames {
		if frames[i].IsLastLayer {
			frames[i].Duration = lastFrameDur
		} else {
			frames[i].Duration = frameDur
		}
	}

	return frames
}

func BratVidToMP4(text string, outputPath string, opts BratVideoOptions) error {
	if opts.OutputFormat == "" {
		opts.OutputFormat = "mp4"
	}

	tokens := TokenizeText(text)
	if len(tokens) == 0 {
		return fmt.Errorf("no tokens found in text")
	}

	frames := BuildFrameSequence(tokens, opts.MaxWordLayer)
	frames = ResolveDurations(frames, opts)

	tmpDir, err := os.MkdirTemp("", "brat-video-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if opts.DebugMode {
		fmt.Printf("[brat] Generated %d frames\n", len(frames))
	}

	for i, frame := range frames {
		framePath := filepath.Join(tmpDir, fmt.Sprintf("frame_%04d.png", i+1))
		imgData, err := BratGen(frame.Text, opts.Theme)
		if err != nil {
			return fmt.Errorf("failed to generate frame %d: %w", i+1, err)
		}

		if err := os.WriteFile(framePath, imgData, 0644); err != nil {
			return fmt.Errorf("failed to write frame %d: %w", i+1, err)
		}

		if opts.DebugMode {
			fmt.Printf("[brat] Generated frame %d: %s (%.2fs)\n", i+1, frame.Text, frame.Duration)
		}
	}

	concatFile := filepath.Join(tmpDir, "concat.txt")
	concatContent := buildFFmpegConcat(tmpDir, frames)
	if err := os.WriteFile(concatFile, []byte(concatContent), 0644); err != nil {
		return fmt.Errorf("failed to write concat file: %w", err)
	}

	var cmd *exec.Cmd
	if opts.OutputFormat == "gif" {
		cmd = exec.Command("ffmpeg",
			"-y",
			"-f", "concat", "-safe", "0", "-i", concatFile,
			"-vf", "fps=10,scale=512:512:flags=lanczos,split[s0][s1],[s0]palettegen=max_colors=64[p],[s1][p]paletteuse=dither=bayer",
			"-loop", "0",
			outputPath,
		)
	} else if opts.OutputFormat == "webp" {
		cmd = exec.Command("ffmpeg",
			"-y",
			"-f", "concat", "-safe", "0", "-i", concatFile,
			"-vcodec", "libwebp", "-lossless", "0", "-compression_level", "5",
			"-q:v", "50", "-loop", "0", "-preset", "picture", "-an", "-vsync", "0",
			"-vf", "scale=512:512,fps=15",
			outputPath,
		)
	} else {
		cmd = exec.Command("ffmpeg",
			"-y",
			"-f", "concat", "-safe", "0", "-i", concatFile,
			"-vf", "scale=512:512",
			"-c:v", "libx264",
			"-preset", "fast",
			"-crf", "18",
			"-pix_fmt", "yuv420p",
			"-movflags", "+faststart",
			outputPath,
		)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg failed: %w", err)
	}

	if opts.DebugMode {
		fmt.Printf("[brat] Video saved to: %s\n", outputPath)
	}

	return nil
}

func buildFFmpegConcat(tmpDir string, frames []BratFrame) string {
	var sb strings.Builder
	for i, frame := range frames {
		framePath := filepath.Join(tmpDir, fmt.Sprintf("frame_%04d.png", i+1))

		escapedPath := strings.ReplaceAll(framePath, "'", "'\\''")
		sb.WriteString(fmt.Sprintf("file '%s'\n", escapedPath))
		sb.WriteString(fmt.Sprintf("duration %.3f\n", frame.Duration))
	}
	return sb.String()
}

func SplitByRegex(text string, pattern string) []string {
	re := regexp.MustCompile(pattern)
	return re.Split(text, -1)
}

func TokenizeWithEmojiPatterns(text string) []string {

	emojiPattern := `[\p{Emoji}]`
	re := regexp.MustCompile(emojiPattern)

	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, ",", " ")

	for strings.Contains(text, "  ") {
		text = strings.ReplaceAll(text, "  ", " ")
	}

	if text == "" {
		return []string{}
	}

	var tokens []string
	words := strings.Fields(text)

	for _, word := range words {

		if re.MatchString(word) {
			parts := re.Split(word, -1)
			emojis := re.FindAllString(word, -1)

			for i, part := range parts {
				if part != "" {
					tokens = append(tokens, part)
				}
				if i < len(emojis) {
					tokens = append(tokens, emojis[i])
				}
			}
		} else {
			tokens = append(tokens, word)
		}
	}

	return tokens
}

func CalculateFrameCount(text string, maxWordsPerLayer int) int {
	tokens := TokenizeText(text)
	frames := BuildFrameSequence(tokens, maxWordsPerLayer)
	return len(frames)
}

func CalculateVideoDuration(text string, maxWordsPerLayer int, opts BratVideoOptions) float64 {
	tokens := TokenizeText(text)
	frames := BuildFrameSequence(tokens, maxWordsPerLayer)
	frames = ResolveDurations(frames, opts)

	var totalDuration float64
	for _, frame := range frames {
		totalDuration += frame.Duration
	}
	return totalDuration
}
