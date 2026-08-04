package ytdlp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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
			if best == nil || f.Height > best.Height || (f.Height == best.Height && f.TBR > best.TBR) {
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
			if best == nil || f.TBR > best.TBR || f.Filesize > best.Filesize {
				best = f
			}
		}
	}
	return best
}

func ExtractInfo(ctx context.Context, targetURL string, extraArgs ...string) (*VideoInfo, error) {
	args := []string{"-J", "--no-warnings"}
	args = append(args, extraArgs...)
	args = append(args, targetURL)

	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
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
