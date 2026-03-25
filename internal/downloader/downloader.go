// Package downloader wraps yt-dlp to fetch video metadata and download files.
package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
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
	return nil
}

// VideoInfo holds the title and available formats for a video.
type VideoInfo struct {
	Title   string
	Formats []FormatInfo
}

// ytDLPMeta is the subset of yt-dlp's -J output we care about.
type ytDLPMeta struct {
	Title   string `json:"title"`
	Formats []struct {
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
// Storyboard (mhtml) entries are excluded.
func FetchVideoInfo(ctx context.Context, url string) (VideoInfo, error) {
	out, err := exec.CommandContext(ctx, "yt-dlp", "-J", "--no-warnings", url).Output()
	if err != nil {
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
			FPS:           f.FPS,
			TBR:           f.TBR,
			VCodec:        f.VCodec,
			AudioChannels: f.AudioChannels,
			Filesize:      size,
			FormatNote:    f.FormatNote,
		})
	}
	return VideoInfo{Title: meta.Title, Formats: formats}, nil
}

// Request holds all parameters needed for a single download.
type Request struct {
	URL    string
	Format Format
	OutDir string
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

	outputTemplate := req.OutDir + "/%(title)s [%(id)s].%(ext)s"

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
		// Write the final filepath to a temp file so we can read it back
		// without mixing it into the progress output.
		"--print-to-file", "after_move:filepath", tmpPath,
	}

	if req.Format.AudioOnly {
		args = append(args,
			"--extract-audio",
			"--audio-format", "mp3",
			"--audio-quality", "0",
		)
	} else {
		args = append(args,
			"--merge-output-format", "mp4",
			"--postprocessor-args", "ffmpeg:-c:a aac",
		)
	}

	args = append(args, req.URL)

	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return Result{}, fmt.Errorf("yt-dlp: %w", err)
	}

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return Result{}, fmt.Errorf("read output path: %w", err)
	}

	return Result{FilePath: strings.TrimSpace(string(data))}, nil
}
