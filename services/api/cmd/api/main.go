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

	apihttp "readvault/api/internal/adapter/http"
	"readvault/api/internal/adapter/postgres"
	"readvault/api/internal/config"
	"readvault/api/internal/importing"
	"readvault/api/internal/library"
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

	handler := apihttp.NewHandler(apihttp.Deps{
		DB:               pool,
		Logger:           logger,
		Imports:          importing.NewService(postgres.NewImportRepository(pool), cfg.MaxImportRecords),
		Library:          library.NewService(postgres.NewLibraryRepository(pool)),
		OwnerID:          postgres.DevOwnerID,
		MaxUploadBytes:   cfg.MaxUploadBytes,
		MaxImportRecords: cfg.MaxImportRecords,
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
