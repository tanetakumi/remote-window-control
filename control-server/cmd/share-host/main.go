// Command share-host is the Windows host of Share App: it serves the web
// client and streams a chosen application window to it, relaying the client's
// touch and keyboard input back to that window.
package main

import (
	"log"

	"share-app-host/internal/app"
	"share-app-host/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	if err := app.Run(cfg); err != nil {
		log.Fatal(err)
	}
}
