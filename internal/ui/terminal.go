// Package ui handles all interactive terminal I/O.
package ui

import (
	"bufio"
	"fmt"
	"io"
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
func (t *Terminal) SelectFormat() (downloader.Format, error) {
	fmt.Fprintln(t.out)
	fmt.Fprintln(t.out, "Download options:")
	for _, f := range downloader.Predefined() {
		fmt.Fprintf(t.out, "  %s) %s\n", f.Key, f.Label)
	}
	fmt.Fprintln(t.out, "  6) Enter format ID manually")

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

	if choice == "6" {
		raw, err := t.Prompt("Format ID (e.g. 137+140 or 22): ")
		if err != nil {
			return downloader.Format{}, err
		}
		if raw == "" {
			return downloader.Format{}, fmt.Errorf("format ID cannot be empty")
		}
		return downloader.ParseCustom(raw)
	}

	return downloader.Format{}, fmt.Errorf("invalid choice %q", choice)
}
