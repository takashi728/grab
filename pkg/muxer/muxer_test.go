package muxer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMuxerCommandGeneration(t *testing.T) {
	m := NewFFmpegMuxer()
	m.GPU.Available = false // Disable GPU for baseline arg test
	args := m.BuildArgs("/tmp/video.mp4", "/tmp/audio.m4a", "/tmp/output.mp4")

	expected := []string{"-y", "-i", "/tmp/video.mp4", "-i", "/tmp/audio.m4a", "-c", "copy", "/tmp/output.mp4"}
	if len(args) != len(expected) {
		t.Fatalf("args length mismatch: expected %d, got %d", len(expected), len(args))
	}
	for i := range args {
		if args[i] != expected[i] {
			t.Errorf("arg %d mismatch: expected %s, got %s", i, expected[i], args[i])
		}
	}
}

func TestMuxerDummyFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "grab_mux_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	videoFile := filepath.Join(tmpDir, "video.mp4")
	audioFile := filepath.Join(tmpDir, "audio.m4a")
	outFile := filepath.Join(tmpDir, "out.mp4")

	_ = os.WriteFile(videoFile, []byte("fake video data"), 0644)
	_ = os.WriteFile(audioFile, []byte("fake audio data"), 0644)

	m := NewFFmpegMuxer()
	if !m.IsAvailable() {
		t.Skip("ffmpeg is not installed on test host, skipping live mux test")
	}

	ctx := context.Background()
	err = m.Mux(ctx, videoFile, audioFile, outFile)
	if err != nil {
		t.Logf("Mux returned error (expected for fake data): %v", err)
	}
}
