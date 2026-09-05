package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

func serve(ctx context.Context, server *http.Server, grace time.Duration) error {
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return err
	}
	return serveListener(ctx, server, listener, grace)
}

func serveListener(ctx context.Context, server *http.Server, listener net.Listener, grace time.Duration) error {
	stopped := make(chan error, 1)
	go func() { stopped <- server.Serve(listener) }()
	select {
	case err := <-stopped:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), grace)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			return errors.Join(err, server.Close())
		}
		if err := <-stopped; !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}
