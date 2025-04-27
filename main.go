package main

import (
	"fmt"
	"os"

	"github.com/alvinunreal/tmuxai/cli"
	"github.com/alvinunreal/tmuxai/logger"
)

func main() {
	// Initialize logger
	if err := logger.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing logger: %v\n", err)
		os.Exit(1)
	}
	logger.Info("TmuxAI starting up")

	// Start the CLI
	if err := cli.Execute(); err != nil {
		logger.Error("Error executing command: %v", err)
		fmt.Fprintf(os.Stderr, "Error executing command: %v\n", err)
		os.Exit(1)
	}
}
