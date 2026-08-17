package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/takashi728/grab/pkg/downloader"
	"github.com/takashi728/grab/pkg/muxer"
	"github.com/takashi728/grab/pkg/ytdlp"
)

// version is set via -ldflags or resolved from the module build info.
var version = ""

func resolveVersion() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return strings.TrimPrefix(bi.Main.Version, "v")
	}
	return "dev"
}

type Config struct {
	URL          string
	OutputPath   string
	Format       string
	Concurrency  int
	NumChunks    int
	GPUIndex     int
	Fallback     bool
	PrintVersion bool
	Verbose      bool
}

func parseFlags() *Config {
	cfg := &Config{}
	flag.StringVar(&cfg.OutputPath, "o", "", "Output filename or path (default: derived from video title)")
	flag.StringVar(&cfg.OutputPath, "output", "", "Output filename or path (default: derived from video title)")
	flag.StringVar(&cfg.Format, "f", "best", "Format selection (best, video-only, audio-only)")
	flag.StringVar(&cfg.Format, "format", "best", "Format selection (best, video-only, audio-only)")
	flag.IntVar(&cfg.Concurrency, "n", 8, "Download concurrency / worker pool size")
	flag.IntVar(&cfg.Concurrency, "concurrency", 8, "Download concurrency / worker pool size")
	flag.IntVar(&cfg.NumChunks, "chunks", 8, "Number of byte-range chunks per file")
	flag.IntVar(&cfg.GPUIndex, "gpu", 0, "GPU index for NVENC/CUDA hardware acceleration (default: 0)")
	flag.BoolVar(&cfg.Fallback, "fallback", false, "Force fallback to standard yt-dlp execution")
	flag.BoolVar(&cfg.PrintVersion, "v", false, "Print version")
	flag.BoolVar(&cfg.PrintVersion, "version", false, "Print version")

	flag.Parse()

	if cfg.PrintVersion {
		fmt.Printf("grab version %s\n", resolveVersion())
		os.Exit(0)
	}

	if flag.NArg() > 0 {
		cfg.URL = flag.Arg(0)
	}

	return cfg
}

func checkRangeSupport(client *http.Client, streamURL string, headers map[string]string) (bool, int64) {
	req, err := http.NewRequest("GET", streamURL, nil)
	if err != nil {
		return false, 0
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Range", "bytes=0-0")

	resp, err := client.Do(req)
	if err != nil {
		return false, 0
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusPartialContent {
		cr := resp.Header.Get("Content-Range")
		if cr != "" {
			parts := strings.Split(cr, "/")
			if len(parts) == 2 {
				if totalSize, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
					return true, totalSize
				}
			}
		}
		return true, resp.ContentLength
	}

	return false, 0
}

func fallbackYTDLP(targetURL string, cfg *Config) error {
	fmt.Println("\x1b[1;33m[*] Range chunking unavailable/disabled. Falling back to native yt-dlp...\x1b[0m")
	ytdlpPath, err := ytdlp.EnsureYTDLPBinary(context.Background())
	if err != nil {
		return fmt.Errorf("yt-dlp binary missing and auto-download failed: %w", err)
	}

	args := []string{}
	if cfg.OutputPath != "" {
		args = append(args, "-o", cfg.OutputPath)
	}
	if cfg.Format != "" && cfg.Format != "best" {
		args = append(args, "-f", cfg.Format)
	}
	args = append(args, targetURL)

	cmd := exec.Command(ytdlpPath, args...)
	var clean []string
	for _, env := range os.Environ() {
		if !strings.HasPrefix(strings.ToLower(env), "ftp_proxy=") {
			clean = append(clean, env)
		}
	}
	cmd.Env = clean
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func main() {
	cfg := parseFlags()

	if cfg.URL == "" {
		fmt.Println("\x1b[1;31mError: missing target URL.\x1b[0m")
		fmt.Println("Usage: grab [options] <URL>")
		flag.PrintDefaults()
		os.Exit(1)
	}

	if cfg.Fallback {
		if err := fallbackYTDLP(cfg.URL, cfg); err != nil {
			fmt.Printf("Fallback failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
		fmt.Println("\n\x1b[1;31mDownload cancelled by user.\x1b[0m")
		os.Exit(130)
	}()

	fmt.Println("\x1b[1;34m[1/4] Fetching video metadata via yt-dlp...\x1b[0m")
	info, err := ytdlp.ExtractInfo(ctx, cfg.URL)
	if err != nil {
		fmt.Printf("\x1b[1;33mWarning: Metadata extraction failed (%v).\x1b[0m\n", err)
		if fErr := fallbackYTDLP(cfg.URL, cfg); fErr != nil {
			os.Exit(1)
		}
		return
	}

	fmt.Printf("\x1b[1mTitle:\x1b[0m %s\n", info.Title)

	var videoFormat, audioFormat *ytdlp.Format

	if cfg.Format == "audio-only" {
		audioFormat = info.BestAudioFormat()
	} else if cfg.Format == "video-only" {
		videoFormat = info.BestVideoFormat()
	} else {
		videoFormat = info.BestVideoFormat()
		audioFormat = info.BestAudioFormat()
	}

	if videoFormat == nil && audioFormat == nil {
		if len(info.Formats) > 0 {
			videoFormat = &info.Formats[len(info.Formats)-1]
		} else {
			fmt.Println("\x1b[1;31mError: No suitable format found.\x1b[0m")
			os.Exit(1)
		}
	}

	client := &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			ResponseHeaderTimeout: 30 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			// googlevideo 403s concurrent range requests when the client
			// advertises Accept-Encoding: gzip; force identity for chunks.
			DisableCompression: true,
		},
		Timeout: 0, // No global timeout for chunk body streaming
	}

	// Prepare output filename
	targetFile := cfg.OutputPath
	if targetFile == "" {
		outputBase := ytdlp.CleanFilename(info.Title)
		ext := info.Ext
		if videoFormat != nil && videoFormat.Ext != "" {
			ext = videoFormat.Ext
		}
		if ext == "" {
			ext = "mp4"
		}
		targetFile = fmt.Sprintf("%s.%s", outputBase, ext)
	}

	muxerImpl := muxer.NewFFmpegMuxer()
	if cfg.GPUIndex >= 0 {
		muxerImpl.GPU.GPUIndex = cfg.GPUIndex
	}

	if muxerImpl.GPU.Available {
		fmt.Printf("\x1b[1;32m[GPU Acceleration Enabled]\x1b[0m %s (CUDA/NVENC device #%d)\n",
			muxerImpl.GPU.Name, muxerImpl.GPU.GPUIndex)
	}

	// Single format or video+audio streams
	if videoFormat != nil && audioFormat != nil && videoFormat.FormatID != audioFormat.FormatID {
		fmt.Println("\x1b[1;34m[2/4] Separate video & audio streams selected. Pre-checking Range support...\x1b[0m")

		vSupportsRange, vTotal := checkRangeSupport(client, videoFormat.URL, videoFormat.HTTPHeaders)
		aSupportsRange, aTotal := checkRangeSupport(client, audioFormat.URL, audioFormat.HTTPHeaders)

		if !vSupportsRange || !aSupportsRange {
			if fErr := fallbackYTDLP(cfg.URL, cfg); fErr != nil {
				os.Exit(1)
			}
			return
		}

		if videoFormat.Filesize <= 0 {
			videoFormat.Filesize = vTotal
		}
		if audioFormat.Filesize <= 0 {
			audioFormat.Filesize = aTotal
		}

		tempDir, err := os.MkdirTemp("", "grab_")
		if err != nil {
			fmt.Printf("Failed to create temp dir: %v\n", err)
			os.Exit(1)
		}
		defer os.RemoveAll(tempDir)

		vExt := videoFormat.Ext
		if vExt == "" {
			vExt = "mp4"
		}
		aExt := audioFormat.Ext
		if aExt == "" {
			aExt = "m4a"
		}

		tempVideoPath := filepath.Join(tempDir, fmt.Sprintf("video_stream.%s", vExt))
		tempAudioPath := filepath.Join(tempDir, fmt.Sprintf("audio_stream.%s", aExt))

		vJob := downloader.NewDownloadJob(videoFormat.URL, tempVideoPath, videoFormat.Filesize, cfg.NumChunks, videoFormat.HTTPHeaders)
		aJob := downloader.NewDownloadJob(audioFormat.URL, tempAudioPath, audioFormat.Filesize, cfg.NumChunks, audioFormat.HTTPHeaders)

		fmt.Println("\x1b[1;34m[3/4] Downloading video stream...\x1b[0m")
		if err := vJob.Execute(ctx, cfg.Concurrency, client, renderProgress(vJob)); err != nil {
			fmt.Printf("\nVideo stream download failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("\n\x1b[1;32mVideo stream downloaded successfully.\x1b[0m")

		fmt.Println("\x1b[1;34m[3/4] Downloading audio stream...\x1b[0m")
		if err := aJob.Execute(ctx, cfg.Concurrency, client, renderProgress(aJob)); err != nil {
			fmt.Printf("\nAudio stream download failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("\n\x1b[1;32mAudio stream downloaded successfully.\x1b[0m")

		fmt.Println("\x1b[1;34m[4/4] Muxing video and audio streams via ffmpeg...\x1b[0m")
		if err := muxerImpl.Mux(ctx, tempVideoPath, tempAudioPath, targetFile); err != nil {
			fmt.Printf("Stream muxing failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("\x1b[1;32mFinished! Saved to %s\x1b[0m\n", targetFile)

	} else {
		targetFormat := videoFormat
		if targetFormat == nil {
			targetFormat = audioFormat
		}

		fmt.Println("\x1b[1;34m[2/4] Checking HTTP Range chunk support...\x1b[0m")
		supportsRange, totalSize := checkRangeSupport(client, targetFormat.URL, targetFormat.HTTPHeaders)
		if !supportsRange {
			if fErr := fallbackYTDLP(cfg.URL, cfg); fErr != nil {
				os.Exit(1)
			}
			return
		}

		if targetFormat.Filesize <= 0 {
			targetFormat.Filesize = totalSize
		}

		job := downloader.NewDownloadJob(targetFormat.URL, targetFile, targetFormat.Filesize, cfg.NumChunks, targetFormat.HTTPHeaders)

		fmt.Println("\x1b[1;34m[3/4] Downloading stream...\x1b[0m")
		if err := job.Execute(ctx, cfg.Concurrency, client, renderProgress(job)); err != nil {
			fmt.Printf("\nDownload failed: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("\n\x1b[1;32mFinished! Saved to %s\x1b[0m\n", targetFile)
	}
}

func renderProgress(job *downloader.DownloadJob) func() {
	var lastRender time.Time
	return func() {
		if time.Since(lastRender) < 100*time.Millisecond {
			return
		}
		lastRender = time.Now()

		downloaded, total, percent := job.GetProgress()
		barWidth := 25
		filled := int((percent / 100.0) * float64(barWidth))
		if filled > barWidth {
			filled = barWidth
		}
		bar := strings.Repeat("=", filled)
		if filled < barWidth {
			bar += ">" + strings.Repeat("-", barWidth-filled-1)
		}

		fmt.Printf("\r\x1b[2KProgress: [%s] \x1b[1;32m%5.1f%%\x1b[0m (%s / %s)",
			bar, percent, downloader.FormatBytes(downloaded), downloader.FormatBytes(total))
	}
}
