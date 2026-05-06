.PHONY: run build tidy lint test docker-build docker-run session-push clean

# VPS host — set via env: VPS_HOST=user@ip make session-push
VPS_HOST ?= user@YOUR_VPS_IP

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

## docker-run: first-time interactive run on VPS (OTP + 2FA prompt)
docker-run:
	docker compose run --rm -it bot

## session-push: copy local session.json → VPS Docker volume (skip OTP on VPS)
## Usage: VPS_HOST=user@1.2.3.4 make session-push
session-push:
	@test -f session.json || (echo "ERROR: session.json not found — run the bot locally first"; exit 1)
	@echo "Pushing session.json to $(VPS_HOST)…"
	ssh $(VPS_HOST) "docker run --rm -v tele-trader_session_data:/data alpine sh -c 'rm -f /data/session.json'"
	cat session.json | ssh $(VPS_HOST) "docker run --rm -i -v tele-trader_session_data:/data alpine sh -c 'cat > /data/session.json && chown 65532:65532 /data/session.json && chmod 600 /data/session.json'"
	@echo "✅ Session pushed. Run: ssh $(VPS_HOST) 'cd ~/path/to/tele-trader && docker compose up -d'"

## lint: run golangci-lint (install: https://golangci-lint.run/usage/install/)
lint:
	golangci-lint run ./...

## clean: remove compiled binaries
clean:
	rm -rf bin/

## help: list available targets
help:
	@grep -E '^## ' Makefile | sed 's/## //'
