package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/opzc35/tuikit/internal/auth"
	"github.com/opzc35/tuikit/internal/sshserver"
	"github.com/opzc35/tuikit/internal/tui"
)

func main() {
	addr := flag.String("addr", ":2222", "SSH listen address")
	dataPath := flag.String("data", "data/users.json", "user database path")
	hostKeyPath := flag.String("host-key", "data/host_key", "SSH host key path")
	flag.Parse()

	store, err := auth.OpenStore(*dataPath)
	if err != nil {
		log.Fatalf("open user store: %v", err)
	}

	if !store.HasAdmin() {
		password := os.Getenv("TUIKIT_ADMIN_PASSWORD")
		if password == "" {
			password = auth.RandomPassword(18)
			log.Printf("generated initial admin password: %s", password)
			log.Print("set TUIKIT_ADMIN_PASSWORD before first run to choose your own password")
		}

		if err := store.CreateUser("admin", password, auth.RoleAdmin); err != nil {
			log.Fatalf("create initial admin: %v", err)
		}
		log.Print("created initial admin user: admin")
	}

	app := tui.New(store)
	server, err := sshserver.New(sshserver.Config{
		Addr:        *addr,
		HostKeyPath: *hostKeyPath,
		Handler:     app.HandleSession,
	})
	if err != nil {
		log.Fatalf("create ssh server: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Printf("TUIkit SSH server listening on %s", *addr)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown error: %v", err)
		}
	case err := <-errCh:
		if err != nil && !errors.Is(err, net.ErrClosed) {
			fmt.Fprintf(os.Stderr, "server error: %v\n", err)
			os.Exit(1)
		}
	}
}
