// Package downloader wraps yt-dlp to fetch video metadata and download files.
package downloader

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
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
// cookiesFromBrowser (e.g. "chrome", "firefox") and cookiesFile (path to a
// Netscape-format cookies.txt) are optional; pass empty strings to skip them.
func FetchVideoInfo(ctx context.Context, url, cookiesFromBrowser, cookiesFile string) (VideoInfo, error) {
	args := []string{"-J", "--no-warnings"}
	args = appendCookieArgs(args, cookiesFromBrowser, cookiesFile)
	args = append(args, url)

	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
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

// appendCookieArgs appends --cookies-from-browser and/or --cookies flags when set.
func appendCookieArgs(args []string, fromBrowser, file string) []string {
	if fromBrowser != "" {
		args = append(args, "--cookies-from-browser", fromBrowser)
	}
	if file != "" {
		args = append(args, "--cookies", file)
	}
	return args
}

// Request holds all parameters needed for a single download.
type Request struct {
	URL          string
	Title        string
	Format       Format
	OutDir       string
	OutputFormat string // container remux: "mp4", "webm", "mkv", etc. Empty = keep original.
	// CookiesFromBrowser passes --cookies-from-browser to yt-dlp (e.g. "chrome", "firefox").
	CookiesFromBrowser string
	// CookiesFile passes --cookies to yt-dlp (path to a Netscape-format cookies.txt).
	CookiesFile string
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

	now := time.Now().Format("01022006_150405")
	sum := sha256.Sum256([]byte(req.Title))
	fileHash := fmt.Sprintf("%x", sum[:8])
	outputTemplate := req.OutDir + "/" + fileHash + "_" + now + ".%(ext)s"

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
	args = appendCookieArgs(args, req.CookiesFromBrowser, req.CookiesFile)

	if req.Format.AudioOnly {
		args = append(args,
			"--extract-audio",
			"--audio-format", "mp3",
			"--audio-quality", "0",
		)
	} else if req.OutputFormat != "" {
		audioCodec := "copy"
		if req.OutputFormat == "mp4" || req.OutputFormat == "mov" || req.OutputFormat == "avi" {
			audioCodec = "aac"
		}
		args = append(args,
			"--merge-output-format", req.OutputFormat,
			"--postprocessor-args", "Merger+ffmpeg_o:-c:v copy -c:a "+audioCodec,
		)
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

	return Result{FilePath: strings.TrimSpace(string(data))}, nil
}
