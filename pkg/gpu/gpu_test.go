package gpu

import (
	"testing"
)

func TestDetectGPU(t *testing.T) {
	info := DetectGPU()
	t.Logf("Detected GPU Info: Available=%v, Name=%s, Count=%d, CUDA=%v",
		info.Available, info.Name, info.Count, info.SupportsCUDA)

	if info.Available {
		if info.Name == "" {
			t.Errorf("expected GPU name when available")
		}
		if info.Count <= 0 {
			t.Errorf("expected count > 0 when GPU available")
		}
	}
}

func TestBuildFFmpegGPUArgs(t *testing.T) {
	info := GPUInfo{
		Available:    true,
		SupportsCUDA: true,
		GPUIndex:     0,
		Name:         "NVIDIA GeForce RTX 5090",
	}

	args := info.FFmpegHWAccelArgs()
	if len(args) == 0 {
		t.Fatalf("expected hardware acceleration args")
	}

	foundCuda := false
	for _, arg := range args {
		if arg == "cuda" {
			foundCuda = true
			break
		}
	}
	if !foundCuda {
		t.Errorf("expected 'cuda' in hwaccel args: %v", args)
	}
}
