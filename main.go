package main

import (
	"fmt"
	"os"

	"github.com/alvinunreal/tmuxai/cli"
	"github.com/alvinunreal/tmuxai/logger"
)

func main() {
	if err := logger.Init(); err != nil {
		// Use fmt.Fprintf here since the logger might not be available.
		fmt.Fprintf(os.Stderr, "Error initializing logger: %v\n", err)
		os.Exit(1)
	}
	// Defer closing the logger to ensure logs are written before exit.
	instance, err := logger.GetInstance()
	if err == nil {
		defer instance.Close()
	}

	// Execute the main CLI command.
	if err := cli.Execute(); err != nil {
		logger.Error("Error executing command: %v", err)
		os.Exit(1)
	}
}
