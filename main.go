package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"downloader/internal/downloader"
	"downloader/internal/storage"
	"downloader/internal/ui"
)

const dbPath = "downloads.db"

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	}))

	db, err := storage.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	ctx := context.Background()

	if err := db.Migrate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	term := ui.New(os.Stdin, os.Stdout)

	if err := run(ctx, term, db); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	_ = logger // available for future verbose mode
}

func run(ctx context.Context, term *ui.Terminal, db *storage.DB) error {
	if err := downloader.CheckDependency(); err != nil {
		return err
	}

	url, err := term.Prompt("Video URL: ")
	if err != nil {
		return err
	}
	if url == "" {
		return fmt.Errorf("URL cannot be empty")
	}

	fmt.Println("Fetching video info...")
	title, err := downloader.GetTitle(ctx, url)
	if err != nil {
		return err
	}
	fmt.Printf("Title: %s\n", title)

	fmt.Println("\nAvailable formats:")
	if err := downloader.ListFormats(ctx, url); err != nil {
		return err
	}

	format, err := term.SelectFormat()
	if err != nil {
		return err
	}

	outDir, err := term.Prompt("\nSave to directory [./downloads]: ")
	if err != nil {
		return err
	}
	if outDir == "" {
		outDir = "./downloads"
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	fmt.Println("\nDownloading...")
	result, err := downloader.Download(ctx, downloader.Request{
		URL:    url,
		Format: format,
		OutDir: outDir,
	})
	if err != nil {
		return err
	}

	record := &storage.Download{
		URL:          url,
		Title:        title,
		FormatArg:    format.Arg,
		QualityLabel: format.Label,
		OutputPath:   result.FilePath,
	}
	if err := db.Save(ctx, record); err != nil {
		// Non-fatal: the file is already on disk.
		fmt.Fprintf(os.Stderr, "warning: failed to save download record: %v\n", err)
	}

	fmt.Printf("\nDone. File saved to: %s\n", result.FilePath)
	return nil
}
