package app

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"share-app-host/internal/auth"
	"share-app-host/internal/config"
	"share-app-host/internal/httpserver"
	"share-app-host/internal/input"
	"share-app-host/internal/nativecapture"
	"share-app-host/internal/signaling"
	"share-app-host/internal/targetwindow"
)

type App struct {
	cfg config.Config
}

func New(cfg config.Config) *App {
	return &App{cfg: cfg}
}

func (a *App) Run() error {
	if err := input.EnableDPIAwareness(); err != nil {
		log.Printf("DPI awareness: %v", err)
	}
	sessions := auth.NewStore(a.cfg.Secret)
	captureBridge := nativecapture.NewBridge(a.cfg.BaseDir)
	targets := targetwindow.NewManager(captureBridge)
	dispatcher := input.NewDispatcher(input.NewSendInputInjector(targets))
	signalingHub := signaling.NewHub(sessions, dispatcher, captureBridge, targets)
	server := httpserver.New(a.cfg.ListenAddr, a.cfg.ClientDir, sessions, signalingHub, captureBridge, targets, a.cfg.BaseDir)
	defer signalingHub.Close()

	printSecretLink(a.cfg.ListenAddr, a.cfg.Secret)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	log.Printf("Serving HTTP on %s", a.cfg.ListenAddr)
	err := server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func printSecretLink(listenAddr, secret string) {
	scheme := "http"
	host := listenAddr
	if address, port, err := net.SplitHostPort(listenAddr); err == nil && (address == "" || address == "0.0.0.0" || address == "::") {
		host = net.JoinHostPort("127.0.0.1", port)
	}

	link := url.URL{
		Scheme: scheme,
		Host:   host,
		Path:   "/",
	}
	query := link.Query()
	query.Set("secret", secret)
	link.RawQuery = query.Encode()

	log.Printf("Local access: %s (use the Windows IP for remote access)", link.String())
}
