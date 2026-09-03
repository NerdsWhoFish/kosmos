.PHONY: dev test build

dev:
	go run ./backend/cmd/kosmos

test:
	go test ./...
	cd frontend && npm ci && npm run build

build:
	go build ./backend/cmd/kosmos
	cd frontend && npm ci && npm run build
