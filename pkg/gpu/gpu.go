package gpu

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type GPUInfo struct {
	Available     bool   `json:"available"`
	Name          string `json:"name"`
	Count         int    `json:"count"`
	GPUIndex      int    `json:"gpu_index"`
	SupportsCUDA  bool   `json:"supports_cuda"`
	SupportsNVENC bool   `json:"supports_nvenc"`
	SupportsQSV   bool   `json:"supports_qsv"`
	SupportsVAAPI bool   `json:"supports_vaapi"`
}

func DetectGPU() GPUInfo {
	info := GPUInfo{
		GPUIndex: 0,
	}

	// 1. Check NVIDIA GPUs via nvidia-smi
	if smiPath, err := exec.LookPath("nvidia-smi"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, smiPath, "--query-gpu=name", "--format=csv,noheader")
		out, err := cmd.Output()
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			var valid []string
			for _, l := range lines {
				l = strings.TrimSpace(l)
				if l != "" {
					valid = append(valid, l)
				}
			}

			if len(valid) > 0 {
				info.Available = true
				info.Count = len(valid)
				info.SupportsCUDA = true
				info.SupportsNVENC = true

				if len(valid) == 1 {
					info.Name = valid[0]
				} else {
					info.Name = fmt.Sprintf("%dx %s", len(valid), valid[0])
				}
			}
		}
	}

	// 2. Check ffmpeg hardware acceleration methods
	if ffPath, err := exec.LookPath("ffmpeg"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, ffPath, "-hwaccels")
		out, err := cmd.Output()
		if err == nil {
			output := strings.ToLower(string(out))
			if strings.Contains(output, "cuda") {
				info.SupportsCUDA = true
			}
			if strings.Contains(output, "qsv") {
				info.SupportsQSV = true
			}
			if strings.Contains(output, "vaapi") {
				info.SupportsVAAPI = true
			}
		}
	}

	return info
}

func (g GPUInfo) FFmpegHWAccelArgs() []string {
	if !g.Available {
		return nil
	}

	if g.SupportsCUDA {
		return []string{
			"-hwaccel", "cuda",
			"-hwaccel_device", strconv.Itoa(g.GPUIndex),
		}
	} else if g.SupportsVAAPI {
		return []string{
			"-hwaccel", "vaapi",
		}
	} else if g.SupportsQSV {
		return []string{
			"-hwaccel", "qsv",
		}
	}

	return nil
}

func (g GPUInfo) NVENCEncoder(vcodec string) string {
	if !g.SupportsNVENC {
		return "libx264"
	}
	vcodec = strings.ToLower(vcodec)
	switch {
	case strings.Contains(vcodec, "av1") || strings.Contains(vcodec, "av01"):
		return "av1_nvenc"
	case strings.Contains(vcodec, "hevc") || strings.Contains(vcodec, "h265"):
		return "hevc_nvenc"
	default:
		return "h264_nvenc"
	}
}
