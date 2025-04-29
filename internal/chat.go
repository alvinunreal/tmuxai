package internal

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/alvinunreal/tmuxai/logger"
	"github.com/chzyer/readline"
	"github.com/fatih/color"
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

	rl, err := readline.NewEx(&readline.Config{
		Prompt:                 c.manager.GetPrompt(),
		HistoryFile:            config.GetConfigFilePath("history"),
		HistorySearchFold:      true,
		InterruptPrompt:        "^C",
		EOFPrompt:              "exit",
		DisableAutoSaveHistory: false,
		AutoComplete:           c.newCompleter(),
	})
	if err != nil {
		return fmt.Errorf("failed to initialize readline: %w", err)
	}
	defer rl.Close()

	if initMessage != "" {
		fmt.Printf("%s%s\n", c.manager.GetPrompt(), initMessage)
		c.processInput(initMessage)
	}

	for {
		rl.SetPrompt(c.manager.GetPrompt())

		line, err := rl.Readline()
		if err == readline.ErrInterrupt {
			// Ctrl+C pressed, clear the line and continue
			continue
		} else if err == io.EOF {
			// Ctrl+D pressed, exit
			return nil
		} else if err != nil {
			return err
		}

		input := strings.TrimSpace(line)
		if input == "exit" || input == "quit" {
			return nil
		}
		if input == "" {
			continue
		}

		logger.Debug("Processing User input: %s", input)
		c.processInput(input)
	}
}

// printWelcomeMessage prints a welcome message
func (c *CLIInterface) printWelcomeMessage() {
	infoColor := color.New(color.FgHiWhite)
	fmt.Println()

	infoColor.Println("Type '/help' for a list of commands, '/exit' to quit")
	fmt.Println()
}

func (c *CLIInterface) processInput(input string) {
	if c.manager.IsMessageSubcommand(input) {
		c.manager.ProcessSubCommand(input)
		return
	}

	// Set up signal handling for Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)

	// Set up a notification channel
	done := make(chan struct{})

	// Launch a goroutine just for handling the interrupt
	go func() {
		select {
		case <-sigChan:
			fmt.Println("canceling...")
			c.manager.Status = ""
			c.manager.WatchMode = false
		case <-done:
		}
	}()

	// Run the message processing in the main thread
	c.manager.Status = "running"
	c.manager.ProcessUserMessage(input)
	c.manager.Status = ""

	close(done)

	signal.Stop(sigChan)
}

// newCompleter creates a readline.AutoCompleter for command completion
func (c *CLIInterface) newCompleter() readline.AutoCompleter {
	configCompleter := readline.PcItem("/config",
		readline.PcItem("set",
			readline.PcItemDynamic(func(_ string) []string {
				// Only return the allowed keys
				return AllowedConfigKeys
			}),
		),
		readline.PcItem("get",
			readline.PcItemDynamic(func(_ string) []string {
				// Only return the allowed keys
				return AllowedConfigKeys
			}),
		),
	)

	// Create completers for each base command using the global subCommands variable
	completers := make([]readline.PrefixCompleterInterface, 0, len(commands))
	for _, cmd := range commands {
		// Special handling for config to add nested completion
		if cmd == "/config" {
			completers = append(completers, configCompleter)
		} else {
			completers = append(completers, readline.PcItem(cmd))
		}
	}

	return readline.NewPrefixCompleter(completers...)
}
