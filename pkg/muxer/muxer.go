package muxer

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/takashi728/grab/pkg/gpu"
)

type Muxer interface {
	IsAvailable() bool
	Mux(ctx context.Context, videoPath, audioPath, outputPath string) error
}

type FFmpegMuxer struct {
	FFmpegPath string
	GPU        gpu.GPUInfo
}

func NewFFmpegMuxer() *FFmpegMuxer {
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		path = "ffmpeg"
	}
	gpuInfo := gpu.DetectGPU()
	return &FFmpegMuxer{
		FFmpegPath: path,
		GPU:        gpuInfo,
	}
}

func (m *FFmpegMuxer) IsAvailable() bool {
	_, err := exec.LookPath(m.FFmpegPath)
	return err == nil
}

func (m *FFmpegMuxer) BuildArgs(videoPath, audioPath, outputPath string) []string {
	args := []string{"-y"}

	// Add GPU hardware acceleration flags if CUDA / VAAPI is available
	if m.GPU.Available {
		hwArgs := m.GPU.FFmpegHWAccelArgs()
		if len(hwArgs) > 0 {
			args = append(args, hwArgs...)
		}
	}

	args = append(args,
		"-i", videoPath,
		"-i", audioPath,
		"-c", "copy",
		outputPath,
	)

	return args
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
