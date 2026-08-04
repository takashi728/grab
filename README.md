# grab

High-performance Go CLI media downloader powered by parallel HTTP range chunking, smart codec selection, and GPU hardware acceleration.

[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue?style=flat-square)](LICENSE)

## Speed First

- **Concurrent Chunking:** $N$ parallel HTTP range workers writing to pre-allocated disk offsets (`WriteAt`).
- **Optimal Efficiency:** Prioritizes **AV1 / VP9** and **Opus** for maximum visual quality at ~40% smaller file size.
- **GPU Accelerated:** Native **NVIDIA CUDA / NVENC** hardware acceleration for `ffmpeg` stream muxing.
- **Zero Bloat:** Single Go binary, zero dependencies, instant fallback to `yt-dlp`.

## Install

```bash
go install github.com/takashi728/grab/cmd/grab@latest
```

or build from source:

```bash
git clone https://github.com/takashi728/grab.git
cd grab && go build -o grab ./cmd/grab
```

## Usage

```bash
# High-speed download
grab "https://www.youtube.com/watch?v=dQw4w9WgXcQ"

# Max speed: 16 parallel workers
grab -n 16 "https://www.youtube.com/watch?v=dQw4w9WgXcQ"

# Audio only (Opus / AAC)
grab -f audio-only "https://www.youtube.com/watch?v=dQw4w9WgXcQ"
```

## CLI Reference

```text
  -o, --output <path>     Output filename
  -f, --format <format>   best | video-only | audio-only (default: best)
  -n, --concurrency <int> Worker pool size (default: 8)
  -gpu <int>              CUDA/NVENC GPU device index (default: 0)
  --fallback              Force native yt-dlp execution
```

## License

MIT
