package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	DefaultAddress    = "127.0.0.1:8787"
	shutdownTimeout   = 30 * time.Second
	readHeaderTimeout = 10 * time.Second
	idleTimeout       = 2 * time.Minute
	maxHeaderBytes    = 1 << 20
)

type Server struct {
	config  Config
	manager *Manager
	handler http.Handler
}

func New(config Config) (*Server, error) {
	config = normalizeConfig(config)
	manager, err := NewManager(config)
	if err != nil {
		return nil, err
	}
	server := &Server{config: config, manager: manager}
	server.handler = server.routes()
	return server, nil
}

func (s *Server) Handler() http.Handler {
	return s.handler
}

func (s *Server) Serve(ctx context.Context, address string) error {
	if strings.TrimSpace(address) == "" {
		address = DefaultAddress
	}
	if err := validateListenAddress(address, s.config.AuthToken); err != nil {
		s.manager.Close()
		return err
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	httpServer := &http.Server{
		Handler:           s.handler,
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.Serve(listener)
	}()

	select {
	case serveErr := <-errCh:
		s.manager.Close()
		if errors.Is(serveErr, http.ErrServerClosed) {
			return nil
		}
		return serveErr
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		shutdownErr := httpServer.Shutdown(shutdownCtx)
		cancel()
		s.manager.Close()
		serveErr := <-errCh
		if shutdownErr != nil {
			return shutdownErr
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			return serveErr
		}
		return nil
	}
}

func validateListenAddress(address string, authToken string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid listen address %q: %w", address, err)
	}
	if isLoopbackHost(host) || strings.TrimSpace(authToken) != "" {
		return nil
	}
	return errors.New("a bearer token is required when listening beyond localhost")
}

func isLoopbackHost(host string) bool {
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
