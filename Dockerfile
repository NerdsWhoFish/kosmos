FROM node:22-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.27-alpine AS backend
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY backend/ ./backend/
RUN CGO_ENABLED=0 go build -o /kosmos ./backend/cmd/kosmos

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=backend /kosmos /kosmos
COPY --from=frontend /src/frontend/dist /web
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/kosmos"]
