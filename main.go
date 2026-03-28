package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"downloader/internal/config"
	"downloader/internal/downloader"
	"downloader/internal/queue"
	"downloader/internal/storage"
	"downloader/internal/ui"
)

func main() {
	cfg := config.Load()

	workerMode := flag.Bool("worker", false, "run as RabbitMQ worker instead of interactive terminal")
	amqpURL := flag.String("amqp", cfg.AMQPUrl, "RabbitMQ connection URL (env: AMQP_URL)")
	outDir := flag.String("out", cfg.OutDir, "output directory for downloads (env: OUT_DIR)")
	dbPath := flag.String("db", cfg.DBPath, "SQLite database file path (env: DB_PATH)")
	flag.Parse()

	level := slog.LevelWarn
	if *workerMode {
		level = slog.LevelInfo
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	db, err := storage.Open(*dbPath)
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

	if *workerMode {
		runWorker(ctx, *amqpURL, *outDir, db, logger)
		return
	}

	term := ui.New(os.Stdin, os.Stdout)
	if err := runCLI(ctx, term, db); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func runWorker(ctx context.Context, amqpURL, outDir string, db *storage.DB, log *slog.Logger) {
	if err := downloader.CheckDependency(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	w, err := queue.NewWorker(amqpURL, db, outDir, log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer w.Close()

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Info("starting worker", "amqp", amqpURL, "out_dir", outDir)
	if err := w.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "worker error: %v\n", err)
		os.Exit(1)
	}
}

func runCLI(ctx context.Context, term *ui.Terminal, db *storage.DB) error {
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
	info, err := downloader.FetchVideoInfo(ctx, url)
	if err != nil {
		return err
	}
	fmt.Printf("Title: %s\n", info.Title)

	format, err := term.SelectFormat(info.Formats)
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
		Title:  info.Title,
		Format: format,
		OutDir: outDir,
	})
	if err != nil {
		return err
	}

	record := &storage.Download{
		URL:          url,
		Title:        info.Title,
		FormatArg:    format.Arg,
		QualityLabel: format.Label,
		OutputPath:   result.FilePath,
	}
	if err := db.Save(ctx, record); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to save download record: %v\n", err)
	}

	fmt.Printf("\nDone. File saved to: %s\n", result.FilePath)
	return nil
}
