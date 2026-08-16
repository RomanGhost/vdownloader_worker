package downloader

import (
	"reflect"
	"testing"
)

func TestAvailableHeights(t *testing.T) {
	cases := []struct {
		name    string
		formats []FormatInfo
		want    []int
	}{
		{
			name:    "no formats",
			formats: nil,
			want:    nil,
		},
		{
			name: "regular YouTube-like ladder",
			formats: []FormatInfo{
				{Height: 360}, {Height: 480}, {Height: 720}, {Height: 1080},
			},
			want: []int{1080, 720, 480, 360},
		},
		{
			// A single 1080p-only format doesn't mean 720/480/360 are also
			// downloadable: bestvideo[height<=720] against a source whose
			// only format is 1080p matches nothing, same as the Instagram
			// case below. Only 1080 itself (and never 1440/2160) is offered.
			name: "never fabricates a tier above the real max, nor below it without a matching format",
			formats: []FormatInfo{
				{Height: 1080},
			},
			want: []int{1080},
		},
		{
			// Instagram reel: only ~1280 and ~1920 exist, nothing at or
			// below 1080/720/480/360. Offering those would make
			// bestvideo[height<=H] match nothing and fail the download.
			name: "sparse heights only offer tiers with a real format at or below",
			formats: []FormatInfo{
				{Height: 1280}, {Height: 1920},
			},
			want: []int{1440},
		},
		{
			// The Height:0 entries (yt-dlp's "unknown height" formats, e.g.
			// Instagram's muxed fallback formats) must not be treated as
			// satisfying every tier's <=H filter - only the real 1920
			// format counts, and nothing in the standard ladder is <=1920
			// while also having a format at or below it.
			name: "unknown height (0) formats are ignored, not treated as fitting every tier",
			formats: []FormatInfo{
				{Height: 0}, {Height: 0}, {Height: 1920},
			},
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := AvailableHeights(tc.formats)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("AvailableHeights(%+v) = %v, want %v", tc.formats, got, tc.want)
			}
		})
	}
}

func TestBuildVideoFormat(t *testing.T) {
	cases := []struct {
		name      string
		height    int
		withAudio bool
		wantArg   string
	}{
		{
			name:      "with audio falls back to a combined format",
			height:    720,
			withAudio: true,
			wantArg:   "bestvideo[height<=720]+bestaudio/best[height<=720]",
		},
		{
			name:      "without audio has no fallback (video-only stream)",
			height:    720,
			withAudio: false,
			wantArg:   "bestvideo[height<=720]",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := BuildVideoFormat(tc.height, tc.withAudio)
			if got.Arg != tc.wantArg {
				t.Errorf("BuildVideoFormat(%d, %v).Arg = %q, want %q", tc.height, tc.withAudio, got.Arg, tc.wantArg)
			}
		})
	}
}

func TestBuildAudioFormat(t *testing.T) {
	cases := []struct {
		name   string
		format string
		want   string
	}{
		{"known format passes through", "opus", "opus"},
		{"unknown format falls back to mp3", "flac", "mp3"},
		{"empty format falls back to mp3", "", "mp3"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := BuildAudioFormat(tc.format)
			if got.AudioFormat != tc.want {
				t.Errorf("BuildAudioFormat(%q).AudioFormat = %q, want %q", tc.format, got.AudioFormat, tc.want)
			}
			if !got.AudioOnly {
				t.Error("BuildAudioFormat(...).AudioOnly = false, want true")
			}
		})
	}
}

func TestParseCustom(t *testing.T) {
	if _, err := ParseCustom("137+140"); err != nil {
		t.Errorf("ParseCustom(\"137+140\") returned unexpected error: %v", err)
	}
	if _, err := ParseCustom("22"); err != nil {
		t.Errorf("ParseCustom(\"22\") returned unexpected error: %v", err)
	}
	if _, err := ParseCustom("; rm -rf /"); err == nil {
		t.Error("ParseCustom(\"; rm -rf /\") did not return an error for an invalid format string")
	}
}
