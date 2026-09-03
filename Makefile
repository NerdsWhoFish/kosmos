.PHONY: dev test build

dev:
	go run ./backend/cmd/kosmos

test:
	GOWORK=off go test ./...
	npm --prefix frontend ci
	npm --prefix frontend test
	npm --prefix frontend run build

build:
	go build ./backend/cmd/kosmos
	cd frontend && npm ci && npm run build
