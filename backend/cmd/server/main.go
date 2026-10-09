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

	"ai-writer/internal/api"
	"ai-writer/internal/config"
	"ai-writer/internal/db"
)

// shutdownTimeout memberi waktu stream yang masih aktif untuk selesai dan menyimpan log.
const shutdownTimeout = 30 * time.Second

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("server berhenti karena error", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	database, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer database.Close()

	results, err := database.Migrate(ctx)
	if err != nil {
		return err
	}
	for _, r := range results {
		logger.Info("migrasi diterapkan", "version", r.Source.Version, "file", r.Source.Path, "duration", r.Duration.String())
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           api.NewRouter(api.Deps{Config: cfg, DB: database, Logger: logger}),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
		// Sengaja tanpa WriteTimeout: stream generate bisa berjalan sampai GENERATE_TIMEOUT.
		ErrorLog: slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("server berjalan", "addr", srv.Addr, "model", cfg.AnthropicModel)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
		close(serveErr)
	}()

	select {
	case err := <-serveErr:
		return fmt.Errorf("listen: %w", err)
	case <-ctx.Done():
	}
	stop() // sinyal kedua langsung mematikan proses

	logger.Info("shutdown dimulai, menunggu request aktif selesai", "timeout", shutdownTimeout.String())
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	logger.Info("server berhenti dengan bersih")
	return nil
}
