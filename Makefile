.PHONY: build run test lint docker-up docker-down

build:
	go build -o bin/time-recording ./cmd/api

run:
	go run ./cmd/api

test:
	go test ./... -v -race -count=1

lint:
	golangci-lint run ./...

docker-up:
	docker compose up --build -d

docker-down:
	docker compose down -v

tidy:
	go mod tidy
