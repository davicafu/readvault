// Command api serves the ReadVault HTTP API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"readvault/api/internal/config"
	"readvault/api/internal/content"
	"readvault/api/internal/content/fetch"
	contenthttp "readvault/api/internal/content/http"
	contentpg "readvault/api/internal/content/postgres"
	"readvault/api/internal/importing"
	importinghttp "readvault/api/internal/importing/http"
	importingpg "readvault/api/internal/importing/postgres"
	"readvault/api/internal/library"
	libraryhttp "readvault/api/internal/library/http"
	librarypg "readvault/api/internal/library/postgres"
	"readvault/api/internal/platform/httpapi"
	"readvault/api/internal/platform/postgres"
)

func main() {
	if err := run(); err != nil {
		slog.Error("api stopped", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	contents := content.NewService(contentpg.NewRepository(pool), fetch.New(fetch.PublicAddr), cfg.MaxArticleBytes,
		content.WithMirror(cfg.ArticleMirrorURL, fetch.MirrorReads(fetch.NewMediumDetector(nil).Is)),
		content.WithArchive(cfg.ArticleArchiveURL))
	owner := postgres.DevOwnerID
	handler := httpapi.NewHandler(httpapi.Deps{
		DB:     pool,
		Logger: logger,
		Modules: []httpapi.Routes{
			importinghttp.New(importing.NewService(importingpg.NewRepository(pool), cfg.MaxImportRecords),
				owner, cfg.MaxUploadBytes, cfg.MaxImportRecords, logger),
			libraryhttp.New(library.NewService(librarypg.NewRepository(pool)), owner, logger),
			contenthttp.New(contents, owner, cfg.MaxArticleBytes, logger),
		},
	})
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           http.TimeoutHandler(handler, cfg.RequestTimeout, `{"error":{"code":"timeout","message":"request timed out"}}`),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       cfg.RequestTimeout,
		WriteTimeout:      cfg.RequestTimeout + 5*time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	// Background text downloads stop with the API: a download cut short is
	// tried again on the next start.
	workerCtx, stopWorker := context.WithCancel(ctx)
	workerDone := make(chan struct{})
	if cfg.DownloadWorkers > 0 {
		worker := content.NewWorker(contents, contentpg.NewDownloadQueue(pool, time.Minute), content.WorkerConfig{Workers: cfg.DownloadWorkers})
		go func() {
			defer close(workerDone)
			worker.Run(workerCtx)
		}()
	} else {
		close(workerDone)
	}
	defer func() {
		stopWorker()
		<-workerDone
	}()

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("api listening", slog.String("addr", cfg.HTTPAddr))
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serving http: %w", err)
		}
		return nil
	case <-ctx.Done():
	}

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutting down http server: %w", err)
	}
	return nil
}
