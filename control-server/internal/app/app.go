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
	"share-app-host/internal/tray"
	"share-app-host/internal/win32"
	"share-app-host/internal/window"
)

const (
	shutdownTimeout = 5 * time.Second
	// statsInterval matches CaptureProbe's dirty-region summaries, so the two
	// log lines can be read side by side.
	statsInterval = 5 * time.Second
)

// Run starts the host and serves until interrupted or exited from the tray.
// It returns nil after a clean shutdown.
func Run(cfg config.Config) error {
	if err := win32.EnableDPIAwareness(); err != nil {
		log.Printf("DPI awareness: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	closeTray, err := tray.Start(stop)
	if err != nil {
		return err
	}
	defer closeTray()

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

	settings := cfg.Settings
	dispatcher := input.NewDispatcher(input.NewMessageInjector(selection, func() float64 {
		return settings.Get().MaxScale
	}))

	dispatcher.SetPCInjector(input.NewSendInputInjector(selection, func() float64 { return settings.Get().MaxScale }))
	hub := session.NewHub(session.Options{
		Dispatcher: dispatcher,
		Source:     PreparingSource(media.ProbeSource(probe, false), PrepareWindow),
		PCSource:   PreparingSource(media.ProbeSource(probe, true), RestoreWindow),
		PreparePC: func(ctx context.Context) error {
			handle, ok := selection.CurrentHandle()
			if !ok {
				return window.ErrNotSelected
			}
			return PreparePCMode(ctx, handle, systemDesktop{})
		},
		Target: selection,
		Encoder: func() media.EncoderConfig {
			encoder := media.DefaultEncoderConfig(cfg.FFmpegPath)
			current := settings.Get()
			encoder.FPS = current.FPS
			encoder.CRF = current.CRF
			return encoder
		},
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
		Settings:  settings,
	})

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP shutdown: %v", err)
		}
	}()
	defer func() {
		stop()
		<-shutdownDone
	}()

	log.Printf("Serving HTTP on %s", cfg.ListenAddr)
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
