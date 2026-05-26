package proxy

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Server struct {
	addr     string
	store    *Store
	server   *http.Server
	done     chan struct{}
	once     sync.Once
}

func NewServer(addr string, store *Store) *Server {
	if addr == "" {
		addr = ":8080"
	}
	return &Server{
		addr:  addr,
		store: store,
		done:  make(chan struct{}),
	}
}

func (s *Server) ListenAndServe() error {
	handler := http.NewServeMux()
	handler.HandleFunc("/", s.handleProxy)

	s.server = &http.Server{
		Addr:         s.addr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}

	log.Printf("API proxy server listening on %s", s.addr)
	defer close(s.done)

	return s.server.Serve(listener)
}

func (s *Server) Shutdown(ctx context.Context) error {
	var err error
	s.once.Do(func() {
		if s.server != nil {
			err = s.server.Shutdown(ctx)
		}
	})
	return err
}

func (s *Server) Addr() string {
	return s.addr
}

func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	route, found := s.store.MatchRoute(r.URL.Path)
	if !found {
		http.Error(w, "no matching route found", http.StatusNotFound)
		return
	}

	// Build the upstream URL
	upstreamPath := r.URL.Path
	if route.PathPrefix != "" {
		upstreamPath = strings.TrimPrefix(upstreamPath, route.PathPrefix)
		if !strings.HasPrefix(upstreamPath, "/") {
			upstreamPath = "/" + upstreamPath
		}
	}
	targetURL := route.Upstream + upstreamPath
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	outReq, err := http.NewRequest(r.Method, targetURL, r.Body)
	if err != nil {
		http.Error(w, "failed to create upstream request", http.StatusInternalServerError)
		return
	}

	// Copy headers from original request, then inject the API key
	for key, values := range r.Header {
		for _, v := range values {
			outReq.Header.Add(key, v)
		}
	}

	if route.APIKey != "" {
		if route.KeyHeader == "Authorization" {
			outReq.Header.Set("Authorization", "Bearer "+route.APIKey)
		} else {
			outReq.Header.Set(route.KeyHeader, route.APIKey)
		}
	}

	// Remove the Host header so it gets set correctly by the transport
	outReq.Host = ""

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(outReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("upstream error: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, v := range values {
			w.Header().Add(key, v)
		}
	}

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}