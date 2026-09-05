package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestShutdownDrainsActiveRequest(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-release:
			_, _ = io.WriteString(w, "saved")
		case <-r.Context().Done():
		}
	})}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer server.Close()
	stopped := make(chan error, 1)
	go func() { stopped <- serveListener(ctx, server, listener, time.Second) }()
	response := make(chan string, 1)
	go func() {
		result, err := http.Get("http://" + listener.Addr().String())
		if err != nil {
			response <- err.Error()
			return
		}
		defer result.Body.Close()
		body, _ := io.ReadAll(result.Body)
		response <- string(body)
	}()
	<-started
	cancel()
	select {
	case err := <-stopped:
		t.Fatalf("server stopped before active request completed: %v", err)
	default:
	}
	close(release)
	if body := <-response; body != "saved" {
		t.Fatalf("active request result = %q", body)
	}
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not finish shutdown")
	}
}

func TestShutdownClosesRequestsAfterDeadline(t *testing.T) {
	started, canceled := make(chan struct{}), make(chan struct{})
	server := &http.Server{Handler: http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(canceled)
	})}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer server.Close()
	stopped := make(chan error, 1)
	go func() { stopped <- serveListener(ctx, server, listener, 20*time.Millisecond) }()
	go func() {
		result, err := http.Get("http://" + listener.Addr().String())
		if err == nil {
			result.Body.Close()
		}
	}()
	<-started
	cancel()
	if err := <-stopped; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown error = %v, want deadline exceeded", err)
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("overdue request was not canceled")
	}
}
