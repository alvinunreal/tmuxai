package internal

import (
	"fmt"
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
		// Allow user to edit the command using readline for better editing experience
		editConfig := &readline.Config{
			Prompt:          "Edit command: ",
			InterruptPrompt: "^C",
			EOFPrompt:       "exit",
		}

		editRl, editErr := readline.NewEx(editConfig)
		if editErr != nil {
			fmt.Printf("Error initializing readline for edit: %v\n", editErr)
			return false, ""
		}
		defer func() { _ = editRl.Close() }()

		// Use ReadlineWithDefault to prefill the command
		editedCommand, editErr := editRl.ReadlineWithDefault(command)
		if editErr != nil {
			if editErr == readline.ErrInterrupt {
				m.Status = ""
				return false, ""
			}

			fmt.Printf("Error reading edited command: %v\n", editErr)
			return false, ""
		}

		editedCommand = strings.TrimSpace(editedCommand)
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
