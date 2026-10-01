package app

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"share-app-host/internal/capture"
	"share-app-host/internal/config"
	"share-app-host/internal/httpserver"
	"share-app-host/internal/input"
	"share-app-host/internal/signaling"
	"share-app-host/internal/win32"
	"share-app-host/internal/window"
)

type App struct {
	cfg config.Config
}

func New(cfg config.Config) *App {
	return &App{cfg: cfg}
}

func (a *App) Run() error {
	if err := win32.EnableDPIAwareness(); err != nil {
		log.Printf("DPI awareness: %v", err)
	}
	captureBridge := capture.NewProbe(a.cfg.ProbePath)
	targets := window.NewSelection(captureBridge)
	dispatcher := input.NewDispatcher(input.NewMessageInjector(targets))
	signalingHub := signaling.NewHub(dispatcher, captureBridge, targets)
	server := httpserver.New(a.cfg.ListenAddr, a.cfg.ClientDir, signalingHub, captureBridge, targets, a.cfg.SnapshotDir)
	defer signalingHub.Close()

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
