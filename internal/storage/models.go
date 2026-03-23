// Package storage manages the SQLite database that records every download.
package storage

import "time"

// Download is a record of a single completed video download.
type Download struct {
	ID           int64
	URL          string    // original video URL
	Title        string    // video title returned by yt-dlp
	FormatArg    string    // value passed to yt-dlp -f flag
	QualityLabel string    // human-readable format description
	OutputPath   string    // absolute path of the file saved to disk
	CreatedAt    time.Time // time the record was inserted
}
