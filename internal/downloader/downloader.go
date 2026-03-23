// Package downloader wraps yt-dlp to fetch video metadata and download files.
package downloader

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// CheckDependency returns an error when yt-dlp is not on PATH.
func CheckDependency() error {
	if _, err := exec.LookPath("yt-dlp"); err != nil {
		return fmt.Errorf(
			"yt-dlp not found on PATH\n" +
				"  install via pip:  pip install yt-dlp\n" +
				"  install via brew: brew install yt-dlp\n" +
				"  releases:         https://github.com/yt-dlp/yt-dlp#installation",
		)
	}
	return nil
}

// GetTitle returns the video title for the given URL.
func GetTitle(ctx context.Context, url string) (string, error) {
	out, err := exec.CommandContext(ctx, "yt-dlp", "--get-title", "--no-warnings", url).Output()
	if err != nil {
		return "", fmt.Errorf("get title: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// ListFormats prints the yt-dlp format table for url to stdout.
func ListFormats(ctx context.Context, url string) error {
	cmd := exec.CommandContext(ctx, "yt-dlp", "-F", "--no-warnings", url)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("list formats: %w", err)
	}
	return nil
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

	args := []string{
		"-f", req.Format.Arg,
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
		args = append(args, "--merge-output-format", "mp4")
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
