package main

import (
	"log"

	"github.com/SPSingh09/zettabridge/internal/adapters/runtime"
)

func main() {
	b, err := runtime.LoadBootstrap()
	if err != nil {
		log.Fatalf("bootstrap: %v", err)
	}
	defer b.PG.Close()
	if err := runtime.RunPaper(b); err != nil {
		log.Fatalf("paper adapter: %v", err)
	}
}
