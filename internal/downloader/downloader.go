// Package downloader wraps yt-dlp to fetch video metadata and download files.
package downloader

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CheckDependency returns an error when yt-dlp or ffmpeg is not on PATH.
func CheckDependency() error {
	if _, err := exec.LookPath("yt-dlp"); err != nil {
		return fmt.Errorf(
			"yt-dlp not found on PATH\n" +
				"  install via pip:  pip install yt-dlp\n" +
				"  install via brew: brew install yt-dlp\n" +
				"  releases:         https://github.com/yt-dlp/yt-dlp#installation",
		)
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf(
			"ffmpeg not found on PATH (required for merging video and audio)\n" +
				"  install via winget: winget install ffmpeg\n" +
				"  install via brew:   brew install ffmpeg\n" +
				"  releases:           https://ffmpeg.org/download.html",
		)
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return fmt.Errorf(
			"ffprobe not found on PATH (required to verify output video codec; ships alongside ffmpeg)",
		)
	}
	return nil
}

// VideoInfo holds the title, duration and available formats for a video.
type VideoInfo struct {
	Title    string
	Duration float64 // seconds; 0 when the source doesn't report it (e.g. livestreams)
	Formats  []FormatInfo
}

// ytDLPMeta is the subset of yt-dlp's -J output we care about.
type ytDLPMeta struct {
	Title    string  `json:"title"`
	Duration float64 `json:"duration"`
	Formats  []struct {
		FormatID       string  `json:"format_id"`
		Ext            string  `json:"ext"`
		Width          int     `json:"width"`
		Height         int     `json:"height"`
		FPS            float64 `json:"fps"`
		TBR            float64 `json:"tbr"`
		VCodec         string  `json:"vcodec"`
		ACodec         string  `json:"acodec"`
		Filesize       int64   `json:"filesize"`
		Resolution     string  `json:"resolution"`
		FilesizeApprox int64   `json:"filesize_approx"`
		AudioChannels  int     `json:"audio_channels"`
		FormatNote     string  `json:"format_note"`
	} `json:"formats"`
}

// FetchVideoInfo calls yt-dlp -J and returns the video title and available formats.
// Storyboard (mhtml) entries are excluded. --no-playlist ensures a URL that's
// part of a playlist/mix/radio (e.g. a YouTube "?list=..." URL) resolves to
// just the single video it points at, not the whole list.
func FetchVideoInfo(ctx context.Context, url string) (VideoInfo, error) {
	cmd := exec.CommandContext(ctx, "yt-dlp", "-J", "--no-warnings", "--no-playlist", url)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return VideoInfo{}, fmt.Errorf("fetch video info: %w\n%s", err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return VideoInfo{}, fmt.Errorf("fetch video info: %w", err)
	}

	var meta ytDLPMeta
	if err := json.Unmarshal(out, &meta); err != nil {
		return VideoInfo{}, fmt.Errorf("parse video info JSON: %w", err)
	}

	formats := make([]FormatInfo, 0, len(meta.Formats))
	for _, f := range meta.Formats {
		if f.Ext == "mhtml" {
			continue
		}
		size := f.Filesize
		if size == 0 {
			size = f.FilesizeApprox
		}
		formats = append(formats, FormatInfo{
			FormatID:      f.FormatID,
			Ext:           f.Ext,
			Resolution:    f.Resolution,
			Height:        f.Height,
			FPS:           f.FPS,
			TBR:           f.TBR,
			VCodec:        f.VCodec,
			AudioChannels: f.AudioChannels,
			Filesize:      size,
			FormatNote:    f.FormatNote,
		})
	}
	return VideoInfo{Title: meta.Title, Duration: meta.Duration, Formats: formats}, nil
}

// Timeout tuning for Download: server-side download speed varies too much to
// estimate actual transfer time, so the budget is a wide multiple of the
// video's own real-time duration (encode/mux/transfer all scale with it)
// rather than a size-based prediction.
const (
	minDownloadTimeout     = 5 * time.Minute
	maxDownloadTimeout     = 3 * time.Hour
	defaultDownloadTimeout = 45 * time.Minute // used when duration is unknown (e.g. livestreams)
	downloadTimeoutFactor  = 3                // worst-case multiple of real-time duration
	downloadTimeoutMargin  = 2 * time.Minute  // fixed overhead: startup, muxing, post-processing
)

// EstimateTimeout returns a generous timeout for downloading and
// post-processing a video of the given duration, bounded so a hung yt-dlp
// process is eventually killed without cutting off legitimately long videos.
func EstimateTimeout(duration time.Duration) time.Duration {
	if duration <= 0 {
		return defaultDownloadTimeout
	}
	t := duration*downloadTimeoutFactor + downloadTimeoutMargin
	switch {
	case t < minDownloadTimeout:
		return minDownloadTimeout
	case t > maxDownloadTimeout:
		return maxDownloadTimeout
	default:
		return t
	}
}

// Request holds all parameters needed for a single download.
type Request struct {
	FileID       string // unique per download; used as the output filename so concurrent jobs never collide
	URL          string
	Title        string
	Format       Format
	OutDir       string
	OutputFormat string // container remux: "mp4", "webm", "mkv", etc. Empty = keep original.
}

// Result contains information about a completed download.
type Result struct {
	// FilePath is the absolute path of the file written to disk.
	FilePath string
}

// Download runs yt-dlp with the given request and returns the path of the
// saved file. Progress is written directly to stdout.
func Download(ctx context.Context, req Request) (Result, error) {
	// yt-dlp will write the final filepath here after any post-processing.
	tmp, err := os.CreateTemp("", "downloader-filepath-*.txt")
	if err != nil {
		return Result{}, fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	// FileID is unique per job, so it can't collide the way a title hash +
	// second-granularity timestamp could when two jobs for the same video
	// are downloaded within the same second (both then race to write/rename
	// the exact same .part file).
	outputTemplate := req.OutDir + "/" + req.FileID + ".%(ext)s"

	formatArg := req.Format.Arg
	if req.Format.MergeAudio {
		formatArg += "+bestaudio"
	}

	args := []string{
		"-f", formatArg,
		"-o", outputTemplate,
		"--no-warnings",
		"--progress",
		"--newline",
		// The output template has no playlist-index placeholder and every
		// job is one file_id -> one file, so a playlist/mix/radio URL must
		// resolve to just the single video it points at.
		"--no-playlist",
		// Write the final filepath to a temp file so we can read it back
		// without mixing it into the progress output.
		"--print-to-file", "after_move:filepath", tmpPath,
	}

	if req.Format.AudioOnly {
		audioFormat := req.Format.AudioFormat
		if audioFormat == "" {
			audioFormat = "mp3"
		}
		args = append(args,
			"--extract-audio",
			"--audio-format", audioFormat,
			"--audio-quality", "0",
		)
	} else if req.OutputFormat != "" {
		// Only takes effect when yt-dlp's Merger postprocessor actually runs
		// (the resolved selector required combining separate video+audio
		// streams). Sites that instead hand back a single already-muxed
		// format (e.g. TikTok's bytevc1) skip Merger entirely, so this alone
		// doesn't guarantee a playable codec — see the ffprobe/ffmpeg pass
		// below, which catches that case too.
		args = append(args, "--merge-output-format", req.OutputFormat)
	}

	args = append(args, req.URL)

	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	cmd.Stdout = os.Stdout
	var stderrBuf bytes.Buffer
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)

	if err := cmd.Run(); err != nil {
		if stderrBuf.Len() > 0 {
			return Result{}, fmt.Errorf("yt-dlp: %w\n%s", err, strings.TrimSpace(stderrBuf.String()))
		}
		return Result{}, fmt.Errorf("yt-dlp: %w", err)
	}

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return Result{}, fmt.Errorf("read output path: %w", err)
	}
	filePath := strings.TrimSpace(string(data))

	// yt-dlp's own --merge-output-format/--recode-video only look at the
	// container (file extension); a source already wrapped in the target
	// container skips their re-encode entirely, whatever codec it carries
	// inside (e.g. TikTok's HEVC-family bytevc1 arrives as .mp4 already).
	// Probe and force a real re-encode ourselves so the codec is guaranteed,
	// not just the extension.
	if !req.Format.AudioOnly && req.OutputFormat != "" {
		compatible, err := hasCompatibleVideoCodec(ctx, filePath, req.OutputFormat)
		if err != nil {
			return Result{}, fmt.Errorf("probe video codec: %w", err)
		}
		if !compatible {
			if err := transcodeVideo(ctx, filePath); err != nil {
				return Result{}, fmt.Errorf("transcode video: %w", err)
			}
		}
	}

	return Result{FilePath: filePath}, nil
}

// containerVideoCodecs lists the video codec ffprobe reports for a source
// already natively compatible with each strict container, so a matching
// probe result can skip re-encoding.
var containerVideoCodecs = map[string]string{
	"mp4": "h264",
	"mov": "h264",
	"avi": "h264",
}

// hasCompatibleVideoCodec reports whether path's video stream is already
// natively compatible with container, via ffprobe.
func hasCompatibleVideoCodec(ctx context.Context, path, container string) (bool, error) {
	want, ok := containerVideoCodecs[container]
	if !ok {
		return true, nil // permissive container (webm, mkv, ...): whatever codec it has is fine
	}
	out, err := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=codec_name",
		"-of", "csv=p=0",
		path,
	).Output()
	if err != nil {
		return false, fmt.Errorf("ffprobe: %w", err)
	}
	return strings.TrimSpace(string(out)) == want, nil
}

// transcodeVideo re-encodes path to H.264/AAC in place, replacing the
// original file once the re-encode succeeds.
func transcodeVideo(ctx context.Context, path string) error {
	tmpOut := path + ".transcode" + filepath.Ext(path)
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", path, "-c:v", "libx264", "-c:a", "aac", tmpOut)
	var stderrBuf bytes.Buffer
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)
	if err := cmd.Run(); err != nil {
		os.Remove(tmpOut)
		if stderrBuf.Len() > 0 {
			return fmt.Errorf("ffmpeg: %w\n%s", err, strings.TrimSpace(stderrBuf.String()))
		}
		return fmt.Errorf("ffmpeg: %w", err)
	}
	return os.Rename(tmpOut, path)
}
