package lib

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func ConvertToOpus(input []byte) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "ffmpeg-conv-*")
	if err != nil {
		return nil, fmt.Errorf("gagal membuat temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	inputPath := filepath.Join(tmpDir, "input.tmp")
	outputPath := filepath.Join(tmpDir, "output.ogg")

	if err := os.WriteFile(inputPath, input, 0644); err != nil {
		return nil, fmt.Errorf("gagal menulis file input: %v", err)
	}

	cmd := exec.Command("ffmpeg",
		"-i", inputPath,
		"-c:a", "libopus",
		"-b:a", "128k",
		"-vbr", "on",
		"-compression_level", "10",
		"-vn",
		outputPath,
	)

	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg error: %v, log: %s", err, errBuf.String())
	}

	outputBytes, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("gagal membaca output ffmpeg: %v", err)
	}

	return outputBytes, nil
}

func ExtractFrameJPEG(input []byte) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "ffmpeg-frame-*")
	if err != nil {
		return nil, fmt.Errorf("gagal membuat temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	inputPath := filepath.Join(tmpDir, "input.tmp")
	outputPath := filepath.Join(tmpDir, "frame.jpg")

	if err := os.WriteFile(inputPath, input, 0644); err != nil {
		return nil, fmt.Errorf("gagal menulis file input: %v", err)
	}

	cmd := exec.Command("ffmpeg",
		"-i", inputPath,
		"-frames:v", "1",
		"-vf", "scale='min(512,iw)':-1",
		"-f", "image2",
		"-vcodec", "mjpeg",
		"-y",
		outputPath,
	)

	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg error: %v, log: %s", err, errBuf.String())
	}

	outputBytes, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("gagal membaca output ffmpeg: %v", err)
	}

	return outputBytes, nil
}
