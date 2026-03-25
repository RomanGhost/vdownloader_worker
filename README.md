# Downloader

A terminal-based video downloader built on top of [yt-dlp](https://github.com/yt-dlp/yt-dlp). Fetches video metadata, presents an interactive format selection menu, downloads the chosen format, and saves a history of all downloads to a local JSON database.

## Requirements

| Tool | Install |
|------|---------|
| [yt-dlp](https://github.com/yt-dlp/yt-dlp) | `pip install yt-dlp` or `winget install yt-dlp` |
| [ffmpeg](https://ffmpeg.org) | `winget install ffmpeg` or `brew install ffmpeg` |
| [Go 1.24+](https://go.dev) | only needed to build from source |

Both `yt-dlp` and `ffmpeg` must be available on `PATH`. The program checks for them at startup and prints installation hints if either is missing.

## Installation

### From source

```bash
git clone <repo-url>
cd Downloader
go build -o downloader .
```

### Docker

```bash
# Build the image
docker build -t downloader .

# Run (downloads saved to ./downloads on the host)
docker run -it -v "$(pwd)/downloads:/downloads" downloader
```

The container includes yt-dlp, ffmpeg, and the compiled binary. No local dependencies needed.

## Usage

```
./downloader
```

The program is fully interactive — no flags required.

1. **Enter a video URL** — any URL supported by yt-dlp (YouTube, Vimeo, Twitter/X, etc.)
2. **Choose a format** from the menu:
   - Predefined presets (best quality, audio-only, 1080p / 720p / 480p)
   - Specific format from a table of all formats available for the video
   - Manual format ID entry (yt-dlp syntax, e.g. `137+140`)
3. **Choose an output directory** (default: `./downloads`)
4. The file is saved as `<title> [<id>].mp4` (or `.mp3` for audio-only)

### Format menu example

```
Predefined:
  1) best video + audio
  2) audio only, best quality
  3) 1080p + audio
  4) 720p + audio
  5) 480p + audio

  No   ID     EXT    RESOLUTION     FPS   AUDIO  SIZE       NOTE
  ────────────────────────────────────────────────────────────────────────
  6    248    webm   1920x1080      30    no     ~          1080p
  7    137    mp4    1920x1080      30    no     ~          1080p
  8    22     mp4    1280x720       30    2ch    ~          720p
  ...
  0) Enter format ID manually

Choice [1]:
```

When a video-only format is selected (no audio), the program asks:
```
Format has no audio. Merge with best audio? [Y/n]:
```

## Output format

- **Video**: merged and re-encoded to **H.264 MP4** with AAC audio for maximum compatibility.
- **Audio-only**: extracted and converted to **MP3** at best quality.

## Download history

Every completed download is appended to `downloads.db` (a JSON file in the working directory). Each record stores:

| Field | Description |
|-------|-------------|
| `id` | Auto-incremented integer |
| `url` | Original video URL |
| `title` | Video title from yt-dlp |
| `format_arg` | Format string passed to yt-dlp `-f` |
| `quality_label` | Human-readable format description |
| `output_path` | Absolute path of the saved file |
| `created_at` | Timestamp of the download |

A failed save to the database is non-fatal — the file is already on disk.

## Project structure

```
.
├── main.go                        # Entry point, wires everything together
├── downloads.db                   # Download history (created on first run)
└── internal/
    ├── downloader/
    │   ├── downloader.go          # yt-dlp wrapper: metadata fetch + download
    │   └── formats.go             # Format types, predefined presets, helpers
    ├── storage/
    │   ├── db.go                  # JSON database: Open, Save, List
    │   └── models.go              # Download record struct
    └── ui/
        └── terminal.go            # Interactive prompts and format table
```
