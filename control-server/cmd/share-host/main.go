// Command share-host is the Windows host of Share App: it serves the web
// client and streams a chosen application window to it, relaying the client's
// touch and keyboard input back to that window.
package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"runtime"

	"share-app-host/internal/app"
	"share-app-host/internal/config"
	"share-app-host/internal/hostlog"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate host executable: %w", err)
	}
	output, err := hostlog.Open(exe)
	if err != nil {
		return err
	}
	defer output.Close()
	log.SetOutput(io.MultiWriter(os.Stderr, output))
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.LUTC)
	log.SetPrefix(fmt.Sprintf("pid=%d ", os.Getpid()))
	log.Printf("host starting executable=%q platform=%s/%s runtime=%s", exe, runtime.GOOS, runtime.GOARCH, runtime.Version())
	cfg, err := config.Load()
	if err != nil {
		log.Printf("configuration failed: %v", err)
		return err
	}
	log.Printf("configuration addr=%q client=%q probe=%q ffmpeg=%q", cfg.ListenAddr, cfg.ClientDir, cfg.ProbePath, cfg.FFmpegPath)
	if err := app.Run(cfg); err != nil {
		log.Printf("host failed: %v", err)
		return err
	}
	log.Print("host stopped")
	return nil
}
