package main

import (
	"log"
	"net/http"
	"os"

	"github.com/pasya/time-recording/internal/db"
	"github.com/pasya/time-recording/internal/handler"
	"github.com/pasya/time-recording/internal/middleware"
	"github.com/pasya/time-recording/internal/repository"
	"github.com/pasya/time-recording/internal/service"
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

	log.Printf("time-recording API listening on :%s", port)
	if err := http.ListenAndServe(":"+port, chain); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
