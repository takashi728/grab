package ytdlp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type Format struct {
	FormatID       string            `json:"format_id"`
	URL            string            `json:"url"`
	Ext            string            `json:"ext"`
	VCodec         string            `json:"vcodec"`
	ACodec         string            `json:"acodec"`
	Filesize       int64             `json:"filesize"`
	FilesizeApprox int64             `json:"filesize_approx"`
	Protocol       string            `json:"protocol"`
	HTTPHeaders    map[string]string `json:"http_headers"`
	Width          int               `json:"width"`
	Height         int               `json:"height"`
	FPS            float64           `json:"fps"`
	TBR            float64           `json:"tbr"`
}

type VideoInfo struct {
	ID                 string            `json:"id"`
	Title              string            `json:"title"`
	Ext                string            `json:"ext"`
	Formats            []Format          `json:"formats"`
	RequestedDownloads []Format          `json:"requested_downloads"`
	HTTPHeaders        map[string]string `json:"http_headers"`
}

func (v *VideoInfo) BestVideoFormat() *Format {
	var best *Format
	for i := range v.Formats {
		f := &v.Formats[i]
		if f.VCodec != "none" && f.VCodec != "" {
			if best == nil {
				best = f
				continue
			}

			// 1. Prefer higher resolution
			if f.Height > best.Height {
				best = f
				continue
			} else if f.Height < best.Height {
				continue
			}

			// 2. Same resolution: prefer higher compression efficiency codec (AV1 > VP9 > H.264)
			fScore := videoCodecScore(f.VCodec)
			bestScore := videoCodecScore(best.VCodec)
			if fScore > bestScore {
				best = f
			} else if fScore == bestScore && f.TBR > best.TBR {
				best = f
			}
		}
	}
	return best
}

func (v *VideoInfo) BestAudioFormat() *Format {
	var best *Format
	for i := range v.Formats {
		f := &v.Formats[i]
		if f.ACodec != "none" && f.ACodec != "" && (f.VCodec == "none" || f.VCodec == "") {
			if best == nil {
				best = f
				continue
			}

			fScore := audioCodecScore(f.ACodec)
			bestScore := audioCodecScore(best.ACodec)
			if fScore > bestScore {
				best = f
			} else if fScore == bestScore && (f.TBR > best.TBR || f.Filesize > best.Filesize) {
				best = f
			}
		}
	}
	return best
}

func videoCodecScore(vcodec string) int {
	vcodec = strings.ToLower(vcodec)
	switch {
	case strings.HasPrefix(vcodec, "av01") || strings.Contains(vcodec, "av1"):
		return 30
	case strings.HasPrefix(vcodec, "vp09") || strings.Contains(vcodec, "vp9"):
		return 20
	case strings.HasPrefix(vcodec, "avc1") || strings.Contains(vcodec, "h264"):
		return 10
	default:
		return 5
	}
}

func audioCodecScore(acodec string) int {
	acodec = strings.ToLower(acodec)
	switch {
	case strings.Contains(acodec, "opus"):
		return 20
	case strings.HasPrefix(acodec, "mp4a") || strings.Contains(acodec, "aac"):
		return 10
	default:
		return 5
	}
}

func ExtractInfo(ctx context.Context, targetURL string, extraArgs ...string) (*VideoInfo, error) {
	ytdlpPath, err := EnsureYTDLPBinary(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to locate or download yt-dlp: %w", err)
	}

	args := []string{"-J", "--no-warnings"}
	args = append(args, extraArgs...)
	args = append(args, targetURL)

	cmd := exec.CommandContext(ctx, ytdlpPath, args...)
	cmd.Env = sanitizeEnv()
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("yt-dlp execution failed (%w): %s", err, string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("failed to run yt-dlp: %w", err)
	}

	var info VideoInfo
	if err := json.Unmarshal(out, &info); err != nil {
		return nil, fmt.Errorf("failed to parse yt-dlp JSON output: %w", err)
	}

	// Fallback headers if format headers are empty
	for i := range info.Formats {
		if len(info.Formats[i].HTTPHeaders) == 0 && len(info.HTTPHeaders) > 0 {
			info.Formats[i].HTTPHeaders = info.HTTPHeaders
		}
	}

	return &info, nil
}

func CleanFilename(title string) string {
	invalid := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	res := title
	for _, inv := range invalid {
		res = strings.ReplaceAll(res, inv, "_")
	}
	return res
}

func EnsureYTDLPBinary(ctx context.Context) (string, error) {
	if path, err := exec.LookPath("yt-dlp"); err == nil {
		return path, nil
	}

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}

	binDir := filepath.Join(cacheDir, "grab", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create bin cache dir: %w", err)
	}

	binName := "yt-dlp"
	if runtime.GOOS == "windows" {
		binName = "yt-dlp.exe"
	}

	targetPath := filepath.Join(binDir, binName)
	if _, err := os.Stat(targetPath); err == nil {
		return targetPath, nil
	}

	// Auto-download standalone yt-dlp binary from official GitHub releases
	downloadURL := "https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp"
	if runtime.GOOS == "windows" {
		downloadURL = "https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp.exe"
	} else if runtime.GOOS == "darwin" {
		downloadURL = "https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp_macos"
	}

	fmt.Printf("\x1b[1;34m[*] yt-dlp not found in PATH. Auto-downloading standalone yt-dlp binary...\x1b[0m\n")

	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create download request for yt-dlp: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download yt-dlp: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download yt-dlp (HTTP %d)", resp.StatusCode)
	}

	tmpPath := targetPath + ".tmp"
	out, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return "", fmt.Errorf("failed to create binary file: %w", err)
	}

	if _, err := io.Copy(out, resp.Body); err != nil {
		out.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to write yt-dlp binary: %w", err)
	}
	out.Close()

	if err := os.Rename(tmpPath, targetPath); err != nil {
		return "", fmt.Errorf("failed to save yt-dlp binary: %w", err)
	}

	_ = os.Chmod(targetPath, 0755)
	return targetPath, nil
}

func sanitizeEnv() []string {
	var clean []string
	for _, env := range os.Environ() {
		if strings.HasPrefix(strings.ToLower(env), "ftp_proxy=") {
			continue
		}
		clean = append(clean, env)
	}
	return clean
}
