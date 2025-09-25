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
	"github.com/nyaosorg/go-readline-ny"
	"github.com/nyaosorg/go-readline-ny/completion"
	"github.com/nyaosorg/go-readline-ny/keys"
	"github.com/nyaosorg/go-readline-ny/simplehistory"
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
	// Check for recent sessions and offer to resume
	if initMessage == "" {
		if sessions, err := c.manager.ListSessions(5); err == nil && len(sessions) > 0 {
			fmt.Println("\nRecent sessions:")
			for i, s := range sessions {
				timeStr := s.UpdatedAt.Format("Jan 2 15:04")
				summary := s.Summary
				if len(summary) > 50 {
					summary = summary[:47] + "..."
				}
				fmt.Printf("%d. %s - %s (%s)\n", i+1, s.Name, timeStr, summary)
			}
			fmt.Println("\nEnter session number to resume, or press Enter for new session:")

			var choice string
			fmt.Scanln(&choice)

			if choice != "" {
				if num := 0; fmt.Sscanf(choice, "%d", &num) == 1 && num > 0 && num <= len(sessions) {
					sessionID := sessions[num-1].ID
					if err := c.manager.LoadSession(sessionID); err == nil {
						fmt.Printf("Resumed session: %s\n", sessions[num-1].Name)
					} else {
						fmt.Printf("Failed to load session: %v\n", err)
					}
				}
			}
		}
	}

	// Create new session if none loaded
	if c.manager.currentSession == nil {
		c.manager.CreateNewSession("New session")
	}

	c.printWelcomeMessage()

	// Initialize history
	history := simplehistory.New()
	historyFilePath := config.GetConfigFilePath("history")

	// Load history from file if it exists
	if historyData, err := os.ReadFile(historyFilePath); err == nil {
		for _, line := range strings.Split(string(historyData), "\n") {
			if line = strings.TrimSpace(line); line != "" {
				history.Add(line)
			}
		}
	}

	// Initialize editor
	editor := &readline.Editor{
		PromptWriter: func(w io.Writer) (int, error) {
			return io.WriteString(w, c.manager.GetPrompt())
		},
		History:        history,
		HistoryCycling: true,
	}

	// Bind TAB key to completion
	editor.BindKey(keys.CtrlI, c.newCompleter())

	if initMessage != "" {
		fmt.Printf("%s%s\n", c.manager.GetPrompt(), initMessage)
		c.processInput(initMessage)
	}

	ctx := context.Background()

	for {
		line, err := editor.ReadLine(ctx)

		if err == readline.CtrlC {
			// Ctrl+C pressed, clear the line and continue
			continue
		} else if err == io.EOF {
			// Ctrl+D pressed, exit
			return nil
		} else if err != nil {
			return err
		}

		// Save history
		if line != "" {
			history.Add(line)

			// Build history data by iterating through all entries
			historyLines := make([]string, 0, history.Len())
			for i := 0; i < history.Len(); i++ {
				historyLines = append(historyLines, history.At(i))
			}
			historyData := strings.Join(historyLines, "\n")
			_ = os.WriteFile(historyFilePath, []byte(historyData), 0644)
		}

		// Process the input (preserving multiline content)
		input := line // Keep the original line including newlines

		// Check for exit/quit commands (only if it's the entire line content)
		trimmed := strings.TrimSpace(input)
		if trimmed == "exit" || trimmed == "quit" {
			return nil
		}
		if trimmed == "" {
			continue
		}

		c.processInput(input)
	}
}

// printWelcomeMessage prints a welcome message
func (c *CLIInterface) printWelcomeMessage() {
	fmt.Println()
	fmt.Println("Type '/help' for a list of commands, '/exit' to quit")
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

// newCompleter creates a completion handler for command completion
func (c *CLIInterface) newCompleter() *completion.CmdCompletionOrList2 {
	return &completion.CmdCompletionOrList2{
		Delimiter: " ",
		Postfix:   " ",
		Candidates: func(field []string) (forComp []string, forList []string) {
			// Handle top-level commands
			if len(field) == 0 || (len(field) == 1 && !strings.HasSuffix(field[0], " ")) {
				return commands, commands
			}

			// Handle /config subcommands
			if len(field) > 0 && field[0] == "/config" {
				if len(field) == 1 || (len(field) == 2 && !strings.HasSuffix(field[1], " ")) {
					return []string{"set", "get"}, []string{"set", "get"}
				} else if len(field) == 2 || (len(field) == 3 && !strings.HasSuffix(field[2], " ")) {
					return AllowedConfigKeys, AllowedConfigKeys
				}
			}

			// Handle /prepare subcommands
			if len(field) > 0 && field[0] == "/prepare" {
				if len(field) == 1 || (len(field) == 2 && !strings.HasSuffix(field[1], " ")) {
					return []string{"bash", "zsh", "fish"}, []string{"bash", "zsh", "fish"}
				}
			}
			return nil, nil
		},
	}
}
