package muxer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

type Muxer interface {
	IsAvailable() bool
	Mux(ctx context.Context, videoPath, audioPath, outputPath string) error
}

type FFmpegMuxer struct {
	FFmpegPath string
}

func NewFFmpegMuxer() *FFmpegMuxer {
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		path = "ffmpeg"
	}
	return &FFmpegMuxer{FFmpegPath: path}
}

func (m *FFmpegMuxer) IsAvailable() bool {
	_, err := exec.LookPath(m.FFmpegPath)
	return err == nil
}

func (m *FFmpegMuxer) BuildArgs(videoPath, audioPath, outputPath string) []string {
	return []string{
		"-y",
		"-i", videoPath,
		"-i", audioPath,
		"-c", "copy",
		outputPath,
	}
}

func (m *FFmpegMuxer) Mux(ctx context.Context, videoPath, audioPath, outputPath string) error {
	if !m.IsAvailable() {
		return fmt.Errorf("ffmpeg binary not found in PATH")
	}

	args := m.BuildArgs(videoPath, audioPath, outputPath)
	cmd := exec.CommandContext(ctx, m.FFmpegMuxerPath(), args...)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg muxing failed (%w): %s", err, string(out))
	}

	// Remove temporary video and audio input streams on success
	_ = os.Remove(videoPath)
	_ = os.Remove(audioPath)

	return nil
}

func (m *FFmpegMuxer) FFmpegMuxerPath() string {
	if m.FFmpegPath != "" {
		return m.FFmpegPath
	}
	return "ffmpeg"
}
