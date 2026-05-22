package sshserver

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

type Session struct {
	User          string
	RemoteAddr    net.Addr
	Stdin         io.Reader
	Stdout        io.Writer
	Stderr        io.Writer
	Env           map[string]string
	WindowChanges <-chan Window
}

type Window struct {
	Width  uint32
	Height uint32
}

type Handler func(context.Context, Session) int

type Config struct {
	Addr        string
	HostKeyPath string
	Handler     Handler
}

type Server struct {
	addr     string
	config   *ssh.ServerConfig
	handler  Handler
	listener net.Listener
	done     chan struct{}
	once     sync.Once
}

func New(cfg Config) (*Server, error) {
	if cfg.Addr == "" {
		cfg.Addr = ":2222"
	}
	if cfg.Handler == nil {
		return nil, errors.New("handler is required")
	}

	signer, err := loadOrCreateHostKey(cfg.HostKeyPath)
	if err != nil {
		return nil, err
	}

	sshConfig := &ssh.ServerConfig{
		NoClientAuth:  true,
		ServerVersion: "SSH-2.0-TUIkit",
	}
	sshConfig.AddHostKey(signer)

	return &Server{
		addr:    cfg.Addr,
		config:  sshConfig,
		handler: cfg.Handler,
		done:    make(chan struct{}),
	}, nil
}

func (s *Server) ListenAndServe() error {
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.listener = listener
	defer close(s.done)

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-s.done:
				return net.ErrClosed
			default:
				return err
			}
		}
		go s.handleConn(conn)
	}
}

func (s *Server) Shutdown(ctx context.Context) error {
	var err error
	s.once.Do(func() {
		if s.listener != nil {
			err = s.listener.Close()
		}
	})

	select {
	case <-s.done:
	case <-ctx.Done():
		if err == nil {
			err = ctx.Err()
		}
	}
	return err
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()

	sshConn, chans, reqs, err := ssh.NewServerConn(conn, s.config)
	if err != nil {
		log.Printf("ssh handshake from %s failed: %v", conn.RemoteAddr(), err)
		return
	}
	defer sshConn.Close()
	go ssh.DiscardRequests(reqs)

	for ch := range chans {
		if ch.ChannelType() != "session" {
			ch.Reject(ssh.UnknownChannelType, "only session channels are supported")
			continue
		}

		channel, requests, err := ch.Accept()
		if err != nil {
			log.Printf("accept channel: %v", err)
			continue
		}
		go s.handleSession(sshConn, channel, requests)
	}
}

func (s *Server) handleSession(conn *ssh.ServerConn, channel ssh.Channel, requests <-chan *ssh.Request) {
	defer channel.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := map[string]string{}
	windows := make(chan Window, 8)
	shellStarted := make(chan struct{})
	var startOnce sync.Once

	go func() {
		defer cancel()
		for req := range requests {
			switch req.Type {
			case "env":
				name, value, ok := parseEnv(req.Payload)
				if ok {
					env[name] = value
				}
				req.Reply(ok, nil)
			case "pty-req":
				win, ok := parsePTY(req.Payload)
				if ok {
					select {
					case windows <- win:
					default:
					}
				}
				req.Reply(ok, nil)
			case "window-change":
				win, ok := parseWindow(req.Payload)
				if ok {
					select {
					case windows <- win:
					default:
					}
				}
			case "shell":
				req.Reply(true, nil)
				startOnce.Do(func() { close(shellStarted) })
			case "exec":
				req.Reply(false, nil)
			default:
				if req.WantReply {
					req.Reply(false, nil)
				}
			}
		}
	}()

	select {
	case <-shellStarted:
	case <-time.After(30 * time.Second):
		fmt.Fprintln(channel.Stderr(), "shell request timed out")
		return
	case <-ctx.Done():
		return
	}

	exitCode := s.handler(ctx, Session{
		User:          conn.User(),
		RemoteAddr:    conn.RemoteAddr(),
		Stdin:         channel,
		Stdout:        channel,
		Stderr:        channel.Stderr(),
		Env:           env,
		WindowChanges: windows,
	})

	channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{uint32(exitCode)}))
}

func loadOrCreateHostKey(path string) (ssh.Signer, error) {
	if path == "" {
		path = "data/host_key"
	}

	key, err := os.ReadFile(path)
	if err == nil {
		return ssh.ParsePrivateKey(key)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	privateKey, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return nil, err
	}

	der := x509.MarshalPKCS1PrivateKey(privateKey)
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: der}
	key = pem.EncodeToMemory(block)
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, err
	}
	return ssh.ParsePrivateKey(key)
}

func parseEnv(payload []byte) (string, string, bool) {
	var req struct {
		Name  string
		Value string
	}
	if err := ssh.Unmarshal(payload, &req); err != nil {
		return "", "", false
	}
	return req.Name, req.Value, true
}

func parsePTY(payload []byte) (Window, bool) {
	var req struct {
		Term        string
		Width       uint32
		Height      uint32
		PixelWidth  uint32
		PixelHeight uint32
		Modes       string
	}
	if err := ssh.Unmarshal(payload, &req); err != nil {
		return Window{}, false
	}
	return Window{Width: req.Width, Height: req.Height}, true
}

func parseWindow(payload []byte) (Window, bool) {
	var req struct {
		Width       uint32
		Height      uint32
		PixelWidth  uint32
		PixelHeight uint32
	}
	if err := ssh.Unmarshal(payload, &req); err != nil {
		return Window{}, false
	}
	return Window{Width: req.Width, Height: req.Height}, true
}
