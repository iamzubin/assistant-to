package main

import (
	"log"
	"os"

	"assistant-to/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		log.Fatalf("Fatal error: %v", err)
		os.Exit(1)
	}
}
