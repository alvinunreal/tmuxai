package system

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/alecthomas/chroma/formatters"
	"github.com/alecthomas/chroma/lexers"
	"github.com/alecthomas/chroma/styles"
	"github.com/alvinunreal/tmuxai/logger"
)

func GetProcessArgs(pid int) string {
	// First check if this is a shell process
	cmd := exec.Command("ps", "-p", fmt.Sprintf("%d", pid), "-o", "command=")
	output, err := cmd.Output()
	if err != nil {
		logger.Error("Failed to get process command for PID %d: %v", pid, err)
		return ""
	}

	cmdOutput := strings.TrimSpace(string(output))

	// If this is a shell process (indicated by starting with "-"),
	// look for child processes that might be the actual command
	if strings.HasPrefix(cmdOutput, "-") {
		// Find child processes
		cmd = exec.Command("pgrep", "-P", fmt.Sprintf("%d", pid))
		childOutput, err := cmd.Output()
		if err == nil && len(childOutput) > 0 {
			// Get the child PIDs
			childPids := strings.Split(strings.TrimSpace(string(childOutput)), "\n")
			if len(childPids) > 0 {
				for _, childPid := range childPids {
					if childPid == "" {
						continue
					}

					// Get the command of this child
					cmd = exec.Command("ps", "-p", childPid, "-o", "command=")
					childCmdOutput, err := cmd.Output()
					if err == nil {
						childCmd := strings.TrimSpace(string(childCmdOutput))
						// If this isn't another shell, return it
						if childCmd != "" && !strings.HasPrefix(childCmd, "-") {
							return childCmd
						}
					}
				}
			}
		}
	} else if cmdOutput != "" && !strings.HasPrefix(cmdOutput, "-") {
		// If it's not a shell and has a command, return it
		return cmdOutput
	}

	// If we couldn't find a better command, return the original
	return cmdOutput
}

var HighlightCode = func(language string, code string) (string, error) {
	// Get the lexer for the specified language
	lexer := lexers.Get(language)
	if lexer == nil {
		lexer = lexers.Analyse(code)
		if lexer == nil {
			lexer = lexers.Fallback
		}
	}

	// Choose a style (theme) - using monokai for good terminal visibility
	style := styles.Get("monokai")
	if style == nil {
		style = styles.Fallback
	}

	// Create a formatter for terminal output (256 colors)
	formatter := formatters.Get("terminal256")
	if formatter == nil {
		formatter = formatters.Fallback
	}

	// Tokenize the code
	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		return "", fmt.Errorf("error tokenizing code: %w", err)
	}

	// Format the code into a string buffer
	var buf bytes.Buffer
	err = formatter.Format(&buf, style, iterator)
	if err != nil {
		return "", fmt.Errorf("error formatting code: %w", err)
	}

	return buf.String(), nil
}

// IsShellCommand checks if the given command is a shell
func IsShellCommand(command string) bool {
	shellCommands := []string{
		"bash", "zsh", "fish", "sh", "dash", "ksh", "csh", "tcsh",
	}
	return slices.Contains(shellCommands, command)
}

func IsSubShell(command string) bool {
	subShellCommands := []string{
		"ssh", "docker", "podman",
	}
	return slices.Contains(subShellCommands, command)
}

func GetOSDetails() string {
	if runtime.GOOS == "linux" {
		// Try reading /etc/os-release
		content, err := os.ReadFile("/etc/os-release")
		if err == nil {
			lines := strings.Split(string(content), "\n")
			info := make(map[string]string)
			for _, line := range lines {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					key := parts[0]
					val := strings.Trim(parts[1], `"`)
					info[key] = val
				}
			}
			// Format without key names
			osName := info["NAME"]
			osVersion := info["VERSION"]
			osID := info["ID"]
			return fmt.Sprintf("%s %s (%s) - %s", osName, osVersion, osID, runtime.GOARCH)
		}
	} else if runtime.GOOS == "darwin" {
		// Use sw_vers command on macOS
		out, err := exec.Command("sw_vers").Output()
		if err == nil {
			// Process the output to create a single line without key names
			outStr := string(out)
			lines := strings.Split(outStr, "\n")
			var osInfo []string

			for _, line := range lines {
				if line == "" {
					continue
				}
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					osInfo = append(osInfo, strings.TrimSpace(parts[1]))
				}
			}

			return strings.Join(osInfo, " ") + " - " + runtime.GOARCH
		}
	}
	return runtime.GOOS + " - " + runtime.GOARCH
}

// structToMap flattens a struct to a map[string]interface{} recursively
func StructToMap(s interface{}, prefix string) map[string]interface{} {
	result := make(map[string]interface{})
	val := reflect.ValueOf(s)
	typ := reflect.TypeOf(s)
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		tag := field.Tag.Get("mapstructure")
		if tag == "" {
			tag = strings.ToLower(field.Name)
		}
		key := tag
		if prefix != "" {
			key = prefix + "." + tag
		}
		fv := val.Field(i)
		if field.Type.Kind() == reflect.Struct {
			nested := StructToMap(fv.Interface(), key)
			for nk, nv := range nested {
				result[nk] = nv
			}
		} else {
			result[key] = fv.Interface()
		}
	}
	return result
}

// setMapValueByDotKey sets a value in a map using dot notation keys
func SetMapValueByDotKey(m map[string]interface{}, key string, value interface{}) {
	m[key] = value
}

// printMapAsYAML prints a map[string]interface{} as YAML-like output with sorted keys
func PrintMapAsYAML(m map[string]interface{}, indent int) {
	// Group keys by prefix for pretty printing
	type node struct {
		val      interface{}
		children map[string]*node
	}
	root := &node{children: map[string]*node{}}
	for k, v := range m {
		parts := strings.Split(k, ".")
		cur := root
		for i, part := range parts {
			if cur.children == nil {
				cur.children = map[string]*node{}
			}
			if _, ok := cur.children[part]; !ok {
				cur.children[part] = &node{}
			}
			cur = cur.children[part]
			if i == len(parts)-1 {
				cur.val = v
			}
		}
	}
	var printNode func(n *node, indent int)
	printNode = func(n *node, indent int) {
		ind := strings.Repeat("  ", indent)

		// Get all keys and sort them
		keys := make([]string, 0, len(n.children))
		for k := range n.children {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		// Iterate through sorted keys
		for _, k := range keys {
			child := n.children[k]
			if len(child.children) > 0 {
				fmt.Printf("%s%s:", ind, k)
				printNode(child, indent+1)
			} else {
				fmt.Printf("%s%s: %v\n", ind, k, child.val)
			}
		}
	}
	printNode(root, indent)
}

// EstimateTokenCount provides a rough estimation of token count for LLM context
// This is an approximation and not exact for all models
func EstimateTokenCount(text string) int {
	// Simple approximation:
	// 1. Split by whitespace for words
	words := strings.Fields(text)
	wordCount := len(words)

	// 2. Count punctuation and special characters which might be separate tokens
	punctCount := 0
	for _, r := range text {
		if unicode.IsPunct(r) || !unicode.IsLetter(r) && !unicode.IsNumber(r) && !unicode.IsSpace(r) {
			punctCount++
		}
	}

	// 3. Estimate: most words are 1 token, some longer words might be multiple tokens
	// Typical ratio is ~0.75 tokens per word for English text + punctuation as separate tokens
	estimatedTokens := int(float64(wordCount)*1.3) + punctCount

	return estimatedTokens
}
