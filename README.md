# grab 🚀

A high-performance CLI media downloader written in Go, acting as a performance-focused wrapper around [`yt-dlp`](https://github.com/yt-dlp/yt-dlp).

`grab` delegates metadata extraction to `yt-dlp`'s JSON interface, then downloads streams in parallel using native HTTP byte-range requests and pre-allocated sparse files for maximum speed and minimal I/O lock contention.

## Features

- **⚡ Fast Byte-Range Downloading**: Splits HTTP media streams into parallel chunk workers writing directly to destination offsets (`WriteAt`).
- **🎬 Automated Stream Muxing**: Downloads separate high-res video and audio DASH streams concurrently and automatically merges them using `ffmpeg`.
- **📊 Real-time Progress Display**: Clean terminal progress reporting showing percentage, byte counts, and speeds.
- **🔄 Smart Fallback**: Automatically falls back to native `yt-dlp` downloading if range chunking is unsupported or restricted by CDN endpoints.
- **🛠 Zero-Bloat CLI**: Simple flags for format selection, concurrency limits, output naming, and chunk count tuning.

## Prerequisites

- [Go 1.26+](https://go.dev/)
- [`yt-dlp`](https://github.com/yt-dlp/yt-dlp) in your system `PATH`
- [`ffmpeg`](https://ffmpeg.org/) in your system `PATH` (optional, for merging separate video & audio streams)

## Installation

### From Source

```bash
git clone https://github.com/takashi728/grab.git
cd grab
go build -o bin/grab ./cmd/grab
```

### Via `go install`

```bash
go install github.com/takashi728/grab/cmd/grab@latest
```

## Usage

```bash
# Basic download
grab "https://www.youtube.com/watch?v=dQw4w9WgXcQ"

# Specify output filename and 16 parallel workers
grab -o output.mp4 -n 16 "https://www.youtube.com/watch?v=dQw4w9WgXcQ"

# Download audio-only format
grab -f audio-only "https://www.youtube.com/watch?v=dQw4w9WgXcQ"

# Force fallback to standard yt-dlp execution
grab --fallback "https://www.youtube.com/watch?v=dQw4w9WgXcQ"
```

## Flags

| Flag | Short | Default | Description |
| --- | --- | --- | --- |
| `--output` | `-o` | `title` | Output filename or path |
| `--format` | `-f` | `best` | Format selection (`best`, `video-only`, `audio-only`) |
| `--concurrency` | `-n` | `8` | Number of parallel worker goroutines |
| `--chunks` | | `8` | Number of byte-range chunks per file |
| `--fallback` | | `false` | Force native `yt-dlp` execution |
| `--version` | `-v` | `false` | Print version |

## Interactive State Prototype

To run the interactive TUI state machine validator:

```bash
go run ./prototype
```

## License

[MIT](LICENSE)
