package main

import (
	"log"

	"github.com/SPSingh09/zettabridge/internal/app"
	"github.com/SPSingh09/zettabridge/internal/config"
)

func main() {
	cfg := config.Load()
	a, err := app.New(cfg)
	if err != nil {
		log.Fatalf("startup: %v", err)
	}
	if a == nil {
		return // MIGRATE_ONLY=true
	}
	a.Run()
}
