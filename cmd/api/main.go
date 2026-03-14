package main

import (
	"context"
	"log"
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
	dsn := getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/timerecording?sslmode=disable")
	port := getEnv("PORT", "8080")

	// Connect and migrate
	database, err := db.Connect(dsn)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer database.Close()

	if err := db.RunMigrations(database); err != nil {
		log.Fatalf("migrations: %v", err)
	}

	// Wire layers
	recordRepo := repository.NewTimeRecordRepository(database)
	calRepo := repository.NewWorkCalendarRepository(database)
	svc := service.NewTimeService(recordRepo, calRepo)
	h := handler.NewTimeHandler(svc)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Apply middleware chain
	chain := middleware.Logger(middleware.RequestID(mux))

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: chain,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("time-recording API listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("server forced to shutdown: %v", err)
	}
	log.Println("server stopped")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
