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
	"ai-writer/internal/auth"
	"ai-writer/internal/config"
	"ai-writer/internal/db"
	"ai-writer/internal/gemini"
	"ai-writer/internal/ratelimit"
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

	// PRD: 5 percobaan login gagal per 15 menit per IP + email.
	loginLimiter := ratelimit.New(5, 15*time.Minute)
	go loginLimiter.RunCleanup(ctx, time.Minute)
	generateLimiter := ratelimit.New(cfg.RateLimitPerMin, time.Minute)
	go generateLimiter.RunCleanup(ctx, time.Minute)

	handler := api.NewRouter(api.Deps{
		Config:          cfg,
		DB:              database,
		Logger:          logger,
		Auth:            auth.NewService(database, cfg.SessionTTL, logger),
		LoginLimiter:    loginLimiter,
		GenerateLimiter: generateLimiter,
		Streamer:        gemini.New(cfg.GeminiAPIKey, cfg.GeminiModel),
	})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
		// Sengaja tanpa WriteTimeout: stream generate bisa berjalan sampai GENERATE_TIMEOUT.
		ErrorLog: slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("server berjalan", "addr", srv.Addr, "model", cfg.GeminiModel, "thinking_level", cfg.GeminiThinkingLevel)
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
