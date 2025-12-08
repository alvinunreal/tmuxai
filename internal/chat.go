package internal

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/ergochat/readline"
)

// Message represents a chat message
type ChatMessage struct {
	Content   string
	FromUser  bool
	Timestamp time.Time
}

type CLIInterface struct {
	manager     *Manager
	initMessage string
	pasteMode   bool
	pasteBuffer strings.Builder
}

func NewCLIInterface(manager *Manager) *CLIInterface {
	return &CLIInterface{
		manager:     manager,
		initMessage: "",
	}
}

// Start starts the CLI interface
func (c *CLIInterface) Start(initMessage string) error {
	c.printWelcomeMessage()

	historyFilePath := config.GetConfigFilePath("history")

	// Initialize readline
	rl, err := readline.NewEx(&readline.Config{
		Prompt:            c.manager.GetPrompt(),
		HistoryFile:       historyFilePath,
		AutoComplete:      NewTmuxAICompleter(c.manager),
		InterruptPrompt:   "^C",
		EOFPrompt:         "exit",
		HistorySearchFold: true,
	})
	if err != nil {
		return err
	}
	defer rl.Close()
	rl.CaptureExitSignal()

	if initMessage != "" {
		fmt.Printf("%s%s\n", c.manager.GetPrompt(), initMessage)
		c.processInput(initMessage)
	}

	for {
		// Update prompt (in case state changed)
		if c.pasteMode {
			rl.SetPrompt("... ")
		} else {
			rl.SetPrompt(c.manager.GetPrompt())
		}

		line, err := rl.Readline()
		if err == readline.ErrInterrupt {
			if len(line) == 0 {
				continue
			} else {
				continue
			}
		} else if err == io.EOF {
			return nil
		} else if err != nil {
			return err
		}

		// Process the input
		input := line

		// Check for exit/quit commands
		trimmed := strings.TrimSpace(input)
		if !c.pasteMode {
			if trimmed == "exit" || trimmed == "quit" {
				return nil
			}
			if trimmed == "" {
				continue
			}
		}

		c.processInput(input)
	}
}

// printWelcomeMessage prints a welcome message
func (c *CLIInterface) printWelcomeMessage() {
	fmt.Println()
	fmt.Println("Type '/help' for a list of commands, '/paste' to enter paste mode, '/exit' to quit")
	fmt.Println()
}

func (c *CLIInterface) processInput(input string) {
	if input == "/paste" {
		c.pasteMode = true
		c.pasteBuffer.Reset()
		fmt.Println("Entering paste mode. Type '/end' to submit, or '/cancel' to abort.")
		return
	}

	if c.pasteMode {
		trimmed := strings.TrimSpace(input)
		if trimmed == "/end" {
			c.pasteMode = false
			input = c.pasteBuffer.String()
			c.pasteBuffer.Reset()
			fmt.Println("Processing pasted content...")
		} else if trimmed == "/cancel" {
			c.pasteMode = false
			c.pasteBuffer.Reset()
			fmt.Println("Paste mode cancelled.")
			return
		} else {
			c.pasteBuffer.WriteString(input + "\n")
			return
		}
	}

	if c.manager.IsMessageSubcommand(input) {
		c.manager.ProcessSubCommand(input)
		return
	}

	// Set up signal handling for Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)

	// Set up a notification channel
	done := make(chan struct{})

	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Launch a goroutine just for handling the interrupt
	go func() {
		select {
		case <-sigChan:
			cancel()
			c.manager.Status = ""
			c.manager.WatchMode = false
		case <-done:
		}
	}()

	// Run the message processing in the main thread
	c.manager.Status = "running"
	c.manager.ProcessUserMessage(ctx, input)
	c.manager.Status = ""

	close(done)

	signal.Stop(sigChan)
}
