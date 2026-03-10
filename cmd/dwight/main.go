package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"dwight/internal/cli"
)

func main() {
	// Create a context that can be cancelled on shutdown signals
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Handle shutdown in a goroutine
	go func() {
		<-sigChan
		log.Println("Received shutdown signal, initiating graceful shutdown...")
		cancel()
	}()

	if err := cli.ExecuteContext(ctx); err != nil {
		log.Fatalf("Fatal error: %v", err)
	}
}
