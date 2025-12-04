package internal

import "strings"

type TmuxAICompleter struct {
	manager *Manager
}

func NewTmuxAICompleter(manager *Manager) *TmuxAICompleter {
	return &TmuxAICompleter{manager: manager}
}

func (c *TmuxAICompleter) Do(line []rune, pos int) (newLine [][]rune, length int) {
	input := string(line[:pos])
	fields := strings.Fields(input)

	// If the cursor is after a space, we are starting a new field
	if len(line) > 0 && line[pos-1] == ' ' {
		fields = append(fields, "")
	}

	var candidates []string

	// Handle top-level commands
	if len(fields) == 0 || (len(fields) == 1 && !strings.HasSuffix(input, " ")) {
		candidates = commands
	} else if len(fields) > 0 {
		switch fields[0] {
		case "/config":
			if len(fields) == 2 {
				candidates = []string{"set", "get"}
			} else if len(fields) == 3 {
				candidates = AllowedConfigKeys
			}
		case "/prepare":
			if len(fields) == 2 {
				candidates = []string{"bash", "zsh", "fish"}
			}
		case "/kb":
			if len(fields) == 2 {
				candidates = []string{"list", "load", "unload"}
			} else if len(fields) == 3 {
				if fields[1] == "load" {
					kbs, err := c.manager.listKBs()
					if err == nil {
						candidates = kbs
					}
				} else if fields[1] == "unload" {
					for name := range c.manager.LoadedKBs {
						candidates = append(candidates, name)
					}
					candidates = append(candidates, "--all")
				}
			}
		case "/model":
			if len(fields) == 2 {
				candidates = c.manager.GetAvailableModels()
			}
		}
	}

	// Filter candidates based on the current word
	var currentWord string
	if len(fields) > 0 {
		currentWord = fields[len(fields)-1]
	}

	for _, candidate := range candidates {
		if strings.HasPrefix(candidate, currentWord) {
			newLine = append(newLine, []rune(candidate[len(currentWord):]))
		}
	}

	return newLine, len(currentWord)
}
