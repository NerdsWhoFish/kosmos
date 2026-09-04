.PHONY: dev dev-backend dev-frontend test build

dev:
	npm --prefix frontend ci
	$(MAKE) -j2 dev-backend dev-frontend

dev-backend:
	go run ./backend/cmd/kosmos

dev-frontend:
	npm --prefix frontend run dev

test:
	GOWORK=off go test ./...
	npm --prefix extensions/kosmos-companion test
	npm --prefix frontend ci
	npm --prefix frontend test
	npm --prefix frontend run build

build:
	go build ./backend/cmd/kosmos
	cd frontend && npm ci --no-audit --no-fund && npm run build
