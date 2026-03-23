package downloader

import (
	"fmt"
	"regexp"
)

// Format describes a yt-dlp format selection.
type Format struct {
	Key       string // menu key shown to the user
	Label     string // human-readable description
	Arg       string // value passed to yt-dlp -f flag
	AudioOnly bool   // triggers --extract-audio post-processing
}

// predefined is the ordered list of built-in format presets.
var predefined = []Format{
	{Key: "1", Label: "best video + audio", Arg: "bestvideo+bestaudio/best"},
	{Key: "2", Label: "audio only, best quality", Arg: "bestaudio/best", AudioOnly: true},
	{Key: "3", Label: "1080p + audio", Arg: "bestvideo[height<=1080]+bestaudio/best[height<=1080]"},
	{Key: "4", Label: "720p + audio", Arg: "bestvideo[height<=720]+bestaudio/best[height<=720]"},
	{Key: "5", Label: "480p + audio", Arg: "bestvideo[height<=480]+bestaudio/best[height<=480]"},
}

// Predefined returns the built-in format presets in menu order.
func Predefined() []Format {
	return predefined
}

// ByKey looks up a format by its menu key.
// The second return value is false when the key is not found.
func ByKey(key string) (Format, bool) {
	for _, f := range predefined {
		if f.Key == key {
			return f, true
		}
	}
	return Format{}, false
}

var customFormatRe = regexp.MustCompile(`^[\d+\s]+$`)

// ParseCustom validates and wraps a raw yt-dlp format string entered by the user.
func ParseCustom(raw string) (Format, error) {
	if !customFormatRe.MatchString(raw) {
		return Format{}, fmt.Errorf(
			"invalid format ID %q: use only digits, '+', and spaces (e.g. 137+140 or 22)",
			raw,
		)
	}
	return Format{
		Key:   "custom",
		Label: "custom: " + raw,
		Arg:   raw,
	}, nil
}
