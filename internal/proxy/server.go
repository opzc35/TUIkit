package proxy

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// hopByHopHeaders are headers that apply to a single transport-level
// connection and must not be forwarded by proxies.
var hopByHopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}

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

const maxRequestBodyBytes = 10 << 20 // 10 MB

func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect || r.Method == http.MethodTrace {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	route, found := s.store.MatchRoute(r.URL.Path)
	if !found {
		http.Error(w, "no matching route found", http.StatusNotFound)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)

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

	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
	if err != nil {
		log.Printf("proxy: failed to create upstream request: %v", err)
		http.Error(w, "failed to create upstream request", http.StatusInternalServerError)
		return
	}

	// Copy headers, skipping hop-by-hop headers.
	for key, values := range r.Header {
		if isHopByHop(key) {
			continue
		}
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
		log.Printf("proxy: upstream request failed: %v", err)
		http.Error(w, "upstream request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers, skipping hop-by-hop headers.
	for key, values := range resp.Header {
		if isHopByHop(key) {
			continue
		}
		for _, v := range values {
			w.Header().Add(key, v)
		}
	}

	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Printf("proxy: error copying response body: %v", err)
	}
}

func isHopByHop(header string) bool {
	for _, h := range hopByHopHeaders {
		if strings.EqualFold(header, h) {
			return true
		}
	}
	return false
}