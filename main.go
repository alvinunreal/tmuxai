package main

import (
	"fmt"
	"os"

	"github.com/alvinunreal/tmuxai/cli"
	"github.com/alvinunreal/tmuxai/config"
	"github.com/alvinunreal/tmuxai/logger"
)

func main() {
	// Initialize logger
	if err := logger.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing logger: %v\n", err)
		os.Exit(1)
	}
	logger.Info("TmuxAI starting up")

	// Initialize configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Error("Error loading configuration: %v", err)
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}
	logger.Info("Configuration loaded successfully")

	// Start the CLI
	if err := cli.Execute(cfg); err != nil {
		logger.Error("Error executing command: %v", err)
		fmt.Fprintf(os.Stderr, "Error executing command: %v\n", err)
		os.Exit(1)
	}
}
