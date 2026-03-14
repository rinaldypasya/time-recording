package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rinaldypasya/time-recording/internal/db"
	"github.com/rinaldypasya/time-recording/internal/handler"
	"github.com/rinaldypasya/time-recording/internal/middleware"
	"github.com/rinaldypasya/time-recording/internal/repository"
	"github.com/rinaldypasya/time-recording/internal/service"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	dsn := getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/timerecording?sslmode=disable")
	port := getEnv("PORT", "8080")
	apiKey := getEnv("API_KEY", "")

	// Connect and migrate
	database, err := db.Connect(dsn)
	if err != nil {
		slog.Error("db connect failed", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	if err := db.RunMigrations(database); err != nil {
		slog.Error("migrations failed", "error", err)
		os.Exit(1)
	}

	// Wire layers
	recordRepo := repository.NewTimeRecordRepository(database)
	calRepo := repository.NewWorkCalendarRepository(database)
	svc := service.NewTimeService(recordRepo, calRepo)
	h := handler.NewTimeHandler(svc)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Apply middleware chain (outermost first)
	// Logger -> RateLimiter -> APIKeyAuth -> RequestID -> mux
	chain := middleware.Logger(
		middleware.RateLimiter(10, 20)(
			middleware.APIKeyAuth(apiKey)(
				middleware.RequestID(mux),
			),
		),
	)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: chain,
	}

	// Start server in a goroutine
	go func() {
		slog.Info("server started", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("shutting down server")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
		os.Exit(1)
	}
	slog.Info("server stopped")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
