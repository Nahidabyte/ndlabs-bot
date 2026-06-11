.PHONY: help build run clean test install-deps fmt vet lint

help:
	@echo "WhatsApp Bot - Available Commands"
	@echo ""
	@echo "  make build         - Build the bot"
	@echo "  make run           - Run the bot"
	@echo "  make clean         - Clean build artifacts"
	@echo "  make test          - Run tests"
	@echo "  make install-deps  - Install Go dependencies"
	@echo "  make fmt           - Format code"
	@echo "  make vet           - Run go vet"
	@echo "  make dev           - Build and run in development mode"

build:
	@echo "Building bot..."
	go build -o wa-bot.exe -v

run: build
	@echo "Running bot..."
	./wa-bot.exe

dev:
	@echo "Running bot in development mode..."
	./wa-bot.exe -prefix "!" -db "wa-bot.db"

clean:
	@echo "Cleaning build artifacts..."
	rm -f wa-bot.exe
	rm -f *.db
	rm -f *.db-shm
	rm -f *.db-wal

test:
	@echo "Running tests..."
	go test -v ./...

install-deps:
	@echo "Installing dependencies..."
	go mod download
	go mod verify

fmt:
	@echo "Formatting code..."
	go fmt ./...

vet:
	@echo "Running go vet..."
	go vet ./...

update-deps:
	@echo "Updating dependencies..."
	go get -u go.mau.fi/whatsmeow
	go mod tidy
