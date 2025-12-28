package internal

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"time"

	"github.com/BoringDystopiaDevelopment/tmuxai/config"
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

	// Bind Ctrl+O and Alt+E to open current prompt in external editor
	editorCmd := &readline.GoCommand{
		Name: "EDIT_IN_EDITOR",
		Func: cmdEditInEditor,
	}
	editor.BindKey(keys.CtrlO, editorCmd)
	editor.BindKey(keys.AltE, editorCmd)

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

// startEditor opens the given text in an external editor and returns the edited result
func startEditor(source string) (string, error) {
	textEditor := os.Getenv("EDITOR")
	if textEditor == "" {
		textEditor = "vim"
	}

	fd, err := os.CreateTemp("", "tmuxai-prompt-*.txt")
	if err != nil {
		return source, err
	}
	fname := fd.Name()
	defer func() { _ = os.Remove(fname) }()

	if _, err := io.WriteString(fd, source); err != nil {
		_ = fd.Close()
		return source, err
	}
	if err := fd.Close(); err != nil {
		return source, err
	}

	cmd := exec.Command(textEditor, fname)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return source, err
	}

	update, err := os.ReadFile(fname)
	if err != nil {
		return source, err
	}
	update = bytes.TrimSuffix(update, []byte{'\n'})
	update = bytes.TrimSuffix(update, []byte{'\r'})
	return string(update), nil
}

// cmdEditInEditor is a readline command that opens the current buffer in an external editor
func cmdEditInEditor(ctx context.Context, B *readline.Buffer) readline.Result {
	result, err := startEditor(B.String())
	if err != nil {
		return readline.CONTINUE
	}
	B.Buffer = B.Buffer[:0]
	B.InsertString(0, result)
	B.Cursor = len(B.Buffer)
	B.RepaintAll()
	return readline.CONTINUE
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

			// Handle /kb subcommands
			if len(field) > 0 && field[0] == "/kb" {
				if len(field) == 1 || (len(field) == 2 && !strings.HasSuffix(field[1], " ")) {
					return []string{"list", "load", "unload"}, []string{"list", "load", "unload"}
				} else if (len(field) == 2 && field[1] == "load") || (len(field) >= 3 && field[1] == "load") {
					// Get available knowledge bases for completion
					kbs, err := c.manager.listKBs()
					if err != nil {
						return nil, nil
					}
					// Disable autocompletion when there's only one KB, bug with readline
					if len(kbs) == 1 {
						return nil, nil
					}
					return kbs, kbs
				} else if (len(field) == 2 && field[1] == "unload") || (len(field) >= 3 && field[1] == "unload") {
					// For unload, show loaded knowledge bases and --all option
					var kbNames []string
					for name := range c.manager.LoadedKBs {
						kbNames = append(kbNames, name)
					}
					kbNames = append(kbNames, "--all")
					return kbNames, kbNames
				}
			}

			// Handle /model subcommands
			if len(field) > 0 && field[0] == "/model" {
				if len(field) == 1 || (len(field) == 2 && !strings.HasSuffix(field[1], " ")) {
					// Return available models for completion
					availableModels := c.manager.GetAvailableModels()
					if len(availableModels) == 0 {
						return nil, nil
					}
					// Disable autocompletion when there's only one model, bug with readline
					if len(availableModels) == 1 {
						return nil, nil
					}
					return availableModels, availableModels
				}
			}
			return nil, nil
		},
	}
}
