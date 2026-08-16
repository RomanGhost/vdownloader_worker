package downloader

import (
	"fmt"
	"regexp"
	"strings"
)

// Format describes a yt-dlp format selection.
type Format struct {
	Key         string // menu key shown to the user
	Label       string // human-readable description
	Arg         string // value passed to yt-dlp -f flag
	AudioOnly   bool   // triggers --extract-audio post-processing
	AudioFormat string // target codec when AudioOnly: "mp3" (default), "m4a", "opus", "wav"
	MergeAudio  bool   // append +bestaudio to Arg before passing to yt-dlp
}

// FormatInfo holds metadata for a single format returned by yt-dlp -J.
type FormatInfo struct {
	FormatID      string
	Ext           string
	Resolution    string // e.g. "1920x1080" or "audio only", from yt-dlp's resolution field
	Height        int    // vertical pixels; 0 for audio-only formats
	FPS           float64
	TBR           float64
	VCodec        string
	AudioChannels int // 0 means no audio (null in JSON)
	Filesize      int64
	FormatNote    string
}

// HasVideo reports whether the format carries a video stream.
func (f FormatInfo) HasVideo() bool {
	return f.VCodec != "" && f.VCodec != "none"
}

// HasAudio reports whether the format carries an audio stream.
func (f FormatInfo) HasAudio() bool {
	return f.AudioChannels > 0
}

// IsAudioOnly reports whether the format has no video stream.
func (f FormatInfo) IsAudioOnly() bool {
	return f.HasAudio() && !f.HasVideo()
}

// IsVideoOnly reports whether the format has no audio stream.
func (f FormatInfo) IsVideoOnly() bool {
	return f.HasVideo() && !f.HasAudio()
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

// ── standardized API quality ladder ──────────────────────────────────────────
//
// The Telegram bot and web UI no longer expose yt-dlp's raw per-video format
// list (it varies wildly by site and isn't meaningful across videos). Instead
// they offer a fixed video-quality ladder, capped to what the source actually
// has, plus a fixed set of audio-extraction targets.

// standardHeights is the video-quality ladder offered to API clients,
// descending. Only tiers at or below the source's actual max height are
// offered — see AvailableHeights.
var standardHeights = []int{2160, 1440, 1080, 720, 480, 360}

// standardAudioFormats is the fixed set of audio-extraction targets offered
// to API clients, default first. Every target is always a transcode via
// ffmpeg, so the set is identical regardless of the source.
var standardAudioFormats = []string{"mp3", "m4a", "opus", "wav"}

// AvailableHeights returns the standard quality tiers achievable for the
// given formats, descending, capped at the source's actual max height. It
// never fabricates a tier the source can't actually deliver (e.g. no 4K
// option is returned for a 1080p source) — including the case where the
// source's heights are sparse (e.g. Instagram reels offering only ~1280 and
// ~1920, nothing in between): a tier is only offered when some format
// actually has height <= that tier, matching yt-dlp's own
// bestvideo[height<=H] selector semantics, since a lower tier with no
// qualifying format would fail downloading with "Requested format is not
// available" instead of falling back to the nearest one below it.
func AvailableHeights(formats []FormatInfo) []int {
	maxHeight := 0
	for _, f := range formats {
		if f.Height > maxHeight {
			maxHeight = f.Height
		}
	}
	hasFormatAtOrBelow := func(h int) bool {
		for _, f := range formats {
			if f.Height > 0 && f.Height <= h {
				return true
			}
		}
		return false
	}
	var out []int
	for _, h := range standardHeights {
		if h <= maxHeight && hasFormatAtOrBelow(h) {
			out = append(out, h)
		}
	}
	return out
}

// StandardAudioFormats returns the fixed list of audio-extraction targets,
// default first.
func StandardAudioFormats() []string {
	return append([]string(nil), standardAudioFormats...)
}

// BuildVideoFormat returns the yt-dlp format selection for a standardized
// quality tier, either muxed with the best available audio track or left
// video-only.
func BuildVideoFormat(height int, withAudio bool) Format {
	if withAudio {
		return Format{
			Arg:   fmt.Sprintf("bestvideo[height<=%d]+bestaudio/best[height<=%d]", height, height),
			Label: fmt.Sprintf("%dp", height),
		}
	}
	return Format{
		Arg:   fmt.Sprintf("bestvideo[height<=%d]", height),
		Label: fmt.Sprintf("%dp (no audio)", height),
	}
}

// BuildAudioFormat returns the Format for extracting audio in the given
// target codec. Unknown or empty values fall back to mp3, the default.
func BuildAudioFormat(audioFormat string) Format {
	switch audioFormat {
	case "m4a", "opus", "wav":
	default:
		audioFormat = "mp3"
	}
	return Format{
		Arg:         "bestaudio/best",
		Label:       strings.ToUpper(audioFormat),
		AudioOnly:   true,
		AudioFormat: audioFormat,
	}
}
