package internal

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/chzyer/readline"
	"github.com/fatih/color"
)

func (m *Manager) confirmedToExecFn(command string, prompt string, edit bool) (bool, string) {
	isSafe, _ := m.whitelistCheck(command)
	if isSafe {
		return true, command
	}

	promptColor := color.New(color.FgCyan, color.Bold)

	var promptText string
	if edit {
		promptText = fmt.Sprintf("%s [Y]es/No/Edit: ", prompt)
	} else {
		promptText = fmt.Sprintf("%s [Y]es/No: ", prompt)
	}

	// Use readline for initial confirmation to properly handle Ctrl+C
	rlConfig := &readline.Config{
		Prompt:          promptColor.Sprint(promptText),
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	}

	rl, err := readline.NewEx(rlConfig)
	if err != nil {
		fmt.Printf("Error initializing readline: %v\n", err)
		return false, ""
	}
	defer func() { _ = rl.Close() }()

	confirmInput, err := rl.Readline()
	if err != nil {
		if err == readline.ErrInterrupt {
			m.Status = ""
			return false, ""
		}

		fmt.Printf("Error reading confirmation: %v\n", err)
		return false, ""
	}

	confirmInput = strings.TrimSpace(strings.ToLower(confirmInput))

	if confirmInput == "" {
		confirmInput = "y"
	}

	switch confirmInput {
	case "y", "yes", "ok", "sure":
		return true, command
	case "e", "edit":
		// Use external editor (Git-like approach)
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = os.Getenv("VISUAL")
		}
		if editor == "" {
			// Fall back to common editors
			editors := []string{"vim", "vi", "nano", "emacs"}
			for _, e := range editors {
				if _, err := exec.LookPath(e); err == nil {
					editor = e
					break
				}
			}
		}

		if editor == "" {
			fmt.Println("Error: No editor found. Please set the EDITOR environment variable.")
			return false, ""
		}

		// Create a temporary file for editing
		tmpFile, err := os.CreateTemp("", "tmuxai-edit-*.sh")
		if err != nil {
			fmt.Printf("Error creating temporary file: %v\n", err)
			return false, ""
		}
		defer os.Remove(tmpFile.Name())

		// Write the command to the temporary file
		if _, err := tmpFile.WriteString(command); err != nil {
			fmt.Printf("Error writing to temporary file: %v\n", err)
			tmpFile.Close()
			return false, ""
		}
		tmpFile.Close()

		// Open the editor
		cmd := exec.Command(editor, tmpFile.Name())
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			fmt.Printf("Error running editor: %v\n", err)
			return false, ""
		}

		// Read the edited command
		editedBytes, err := os.ReadFile(tmpFile.Name())
		if err != nil {
			fmt.Printf("Error reading edited command: %v\n", err)
			return false, ""
		}

		editedCommand := strings.TrimSpace(string(editedBytes))
		if editedCommand != "" {
			return true, editedCommand
		} else {
			// empty command
			return false, ""
		}
	case "n", "no", "cancel":
		return false, ""
	default:
		// any other input is retry confirmation
		return m.confirmedToExecFn(command, prompt, edit)
	}
}

func (m *Manager) whitelistCheck(command string) (bool, error) {
	isWhitelisted := false
	for _, pattern := range m.Config.WhitelistPatterns {
		if pattern == "" {
			continue
		}
		match, err := regexp.MatchString(pattern, command)
		if err != nil {
			return false, fmt.Errorf("invalid whitelist regex pattern '%s': %w", pattern, err)
		}
		if match {
			isWhitelisted = true
			break
		}
	}

	if !isWhitelisted {
		return false, nil
	}

	for _, pattern := range m.Config.BlacklistPatterns {
		if pattern == "" {
			continue
		}
		match, err := regexp.MatchString(pattern, command)
		if err != nil {
			return false, fmt.Errorf("invalid blacklist regex pattern '%s': %w", pattern, err)
		}
		if match {
			return false, nil
		}
	}

	return true, nil
}
