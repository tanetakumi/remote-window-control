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
	if err := app.New(cfg).Run(); err != nil {
		log.Fatal(err)
	}
}
