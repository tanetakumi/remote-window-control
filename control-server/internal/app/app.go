// Package app wires the host's components together and runs them.
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
	"share-app-host/internal/httpapi"
	"share-app-host/internal/input"
	"share-app-host/internal/media"
	"share-app-host/internal/rdpkeep"
	"share-app-host/internal/session"
	"share-app-host/internal/win32"
	"share-app-host/internal/window"
)

const (
	shutdownTimeout = 5 * time.Second
	// statsInterval matches CaptureProbe's dirty-region summaries, so the two
	// log lines can be read side by side.
	statsInterval = 5 * time.Second
)

// Run starts the host and serves until interrupted. It returns nil after a
// clean shutdown.
func Run(cfg config.Config) error {
	if err := win32.EnableDPIAwareness(); err != nil {
		log.Printf("DPI awareness: %v", err)
	}

	measuring := cfg.CaptureStats != config.CaptureStatsOff
	probe := capture.NewProbe(cfg.ProbePath, capture.StreamOptions{
		Stats:       measuring,
		VerifyStats: cfg.CaptureStats == config.CaptureStatsVerify,
	})
	var mediaStats time.Duration
	if measuring {
		mediaStats = statsInterval
		log.Printf("capture measurement stats=%s", cfg.CaptureStats)
	}
	selection := window.NewSelection(probe)
	dispatcher := input.NewDispatcher(input.NewMessageInjector(selection))

	encoder := media.DefaultEncoderConfig(cfg.FFmpegPath)
	encoder.FPS = cfg.FPS

	hub := session.NewHub(session.Options{
		Dispatcher:    dispatcher,
		Source:        PreparingSource(media.ProbeSource(probe), PrepareWindow),
		Target:        selection,
		Encoder:       encoder,
		StatsInterval: mediaStats,
	})
	defer hub.Close()

	keeper := rdpkeep.NewManager(rdpkeep.Config{
		Username: cfg.RDPUsername,
		Password: cfg.RDPPassword,
	})
	defer keeper.Stop()

	server := httpapi.New(httpapi.Options{
		Addr:      cfg.ListenAddr,
		ClientDir: cfg.ClientDir,
		Windows:   NewTargetService(selection, dispatcher),
		Snapshots: probe,
		Control:   hub,
		Keepalive: keeper,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	log.Printf("Serving HTTP on %s", cfg.ListenAddr)
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
