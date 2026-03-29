package downloader

import (
	"fmt"
	"regexp"
)

// Format describes a yt-dlp format selection.
type Format struct {
	Key        string // menu key shown to the user
	Label      string // human-readable description
	Arg        string // value passed to yt-dlp -f flag
	AudioOnly  bool   // triggers --extract-audio post-processing
	MergeAudio bool   // append +bestaudio to Arg before passing to yt-dlp
}

// FormatInfo holds metadata for a single format returned by yt-dlp -J.
type FormatInfo struct {
	FormatID      string
	Ext           string
	Resolution    string // e.g. "1920x1080" or "audio only", from yt-dlp's resolution field
	FPS           float64
	TBR           float64
	VCodec        string
	AudioChannels int // 0 means no audio (null in JSON)
	Filesize      int64
	FormatNote    string
}

// IsAudioOnly reports whether the format has no video stream.
func (f FormatInfo) IsAudioOnly() bool {
	return f.VCodec == "none" || f.VCodec == ""
}

// IsVideoOnly reports whether the format has no audio stream.
func (f FormatInfo) IsVideoOnly() bool {
	return f.AudioChannels == 0
}

// FilesizeStr returns a human-readable filesize.
func (f FormatInfo) FilesizeStr() string {
	if f.Filesize == 0 {
		return "~"
	}
	return fmt.Sprintf("%.2fMiB", float64(f.Filesize)/(1024*1024))
}

// ToFormat converts FormatInfo into a Format ready for downloading.
func (f FormatInfo) ToFormat() Format {
	label := f.FormatID
	if f.FormatNote != "" {
		label += " " + f.FormatNote
	}
	return Format{
		Key:        f.FormatID,
		Label:      label,
		Arg:        f.FormatID,
		AudioOnly:  f.IsAudioOnly(),
		MergeAudio: f.IsVideoOnly(),
	}
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

// ByKey looks up a predefined format by its menu key.
// The second return value is false when the key is not found.
func ByKey(key string) (Format, bool) {
	for _, f := range predefined {
		if f.Key == key {
			return f, true
		}
	}
	return Format{}, false
}

// outputFormats is the list of containers supported for remuxing via ffmpeg.
var outputFormats = []string{"mp4", "webm", "mkv", "mov", "avi"}

// OutputFormats returns the containers available for --merge-output-format.
func OutputFormats() []string { return outputFormats }

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
