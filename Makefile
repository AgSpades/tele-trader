.PHONY: run build tidy lint test docker-build docker-run clean

# Default target
all: build

## run: start the bot (requires .env)
run:
	go run ./cmd/bot/...

## build: compile the bot binary
build:
	go build -o bin/tele-trader ./cmd/bot/...

## tidy: tidy go modules
tidy:
	go mod tidy

## test: run all unit tests
test:
	go test ./... -v -count=1

## docker-build: build the Docker image
docker-build:
	DOCKER_BUILDKIT=1 docker build -t tele-trader:latest .

## docker-run: first-time interactive run (OTP + 2FA prompt)
docker-run:
	docker compose run --rm -it bot

## lint: run golangci-lint (install: https://golangci-lint.run/usage/install/)
lint:
	golangci-lint run ./...

## clean: remove compiled binaries
clean:
	rm -rf bin/

## help: list available targets
help:
	@grep -E '^## ' Makefile | sed 's/## //'
