// Package ui handles all interactive terminal I/O.
package ui

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"downloader/internal/downloader"
)

// Terminal wraps a reader/writer pair for interactive prompts.
type Terminal struct {
	reader *bufio.Reader
	out    io.Writer
}

// New creates a Terminal backed by r and w.
func New(r io.Reader, w io.Writer) *Terminal {
	return &Terminal{
		reader: bufio.NewReader(r),
		out:    w,
	}
}

// Prompt writes label to the output and returns the trimmed line the user types.
func (t *Terminal) Prompt(label string) (string, error) {
	fmt.Fprint(t.out, label)
	line, err := t.reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read input: %w", err)
	}
	return strings.TrimSpace(line), nil
}

// SelectFormat prints the format menu and returns the chosen Format.
// available is the list of formats fetched from yt-dlp -J.
func (t *Terminal) SelectFormat(available []downloader.FormatInfo) (downloader.Format, error) {
	fmt.Fprintln(t.out)
	fmt.Fprintln(t.out, "Predefined:")
	for _, f := range downloader.Predefined() {
		fmt.Fprintf(t.out, "  %s) %s\n", f.Key, f.Label)
	}

	if len(available) > 0 {
		fmt.Fprintln(t.out)
		fmt.Fprintf(t.out, "  %-4s %-6s %-6s %-14s %-5s %-6s %-10s %s\n",
			"No", "ID", "EXT", "RESOLUTION", "FPS", "AUDIO", "SIZE", "NOTE")
		fmt.Fprintln(t.out, "  "+strings.Repeat("─", 68))
		for i, f := range available {
			fps := ""
			if f.FPS > 0 {
				fps = fmt.Sprintf("%.0f", f.FPS)
			}
			audio := "no"
			if f.AudioChannels > 0 {
				audio = fmt.Sprintf("%dch", f.AudioChannels)
			}
			fmt.Fprintf(t.out, "  %-4d %-6s %-6s %-14s %-5s %-6s %-10s %s\n",
				i+6, f.FormatID, f.Ext, f.Resolution, fps, audio, f.FilesizeStr(), f.FormatNote)
		}
	}

	fmt.Fprintln(t.out, "  0) Enter format ID manually")

	choice, err := t.Prompt("\nChoice [1]: ")
	if err != nil {
		return downloader.Format{}, err
	}
	if choice == "" {
		choice = "1"
	}

	if f, ok := downloader.ByKey(choice); ok {
		return f, nil
	}

	if choice == "0" {
		raw, err := t.Prompt("Format ID (e.g. 137+140 or 22): ")
		if err != nil {
			return downloader.Format{}, err
		}
		if raw == "" {
			return downloader.Format{}, fmt.Errorf("format ID cannot be empty")
		}
		return downloader.ParseCustom(raw)
	}

	if n, err := strconv.Atoi(choice); err == nil {
		idx := n - 6
		if idx >= 0 && idx < len(available) {
			f := available[idx].ToFormat()
			if f.MergeAudio {
				merge, err := t.Prompt("Format has no audio. Merge with best audio? [Y/n]: ")
				if err != nil {
					return downloader.Format{}, err
				}
				f.MergeAudio = merge == "" || merge == "y" || merge == "Y"
			}
			return f, nil
		}
	}

	return downloader.Format{}, fmt.Errorf("invalid choice %q", choice)
}
