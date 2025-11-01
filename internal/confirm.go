package internal

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/fatih/color"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

func (m *Manager) confirmedToExecFn(command string, prompt string, edit bool) (bool, string) {
	isSafe, _ := m.whitelistCheck(command)
	if isSafe {
		return true, command
	}

	promptColor := color.New(color.FgCyan, color.Bold)

	// Score the command for risk assessment
	assessment := ScoreCommand(command)

	// Determine color and icon based on risk level
	var riskColor *color.Color
	var riskIcon string
	switch assessment.Level {
	case RiskDanger:
		riskColor = color.New(color.FgRed, color.Bold)
		riskIcon = "!"
	case RiskUnknown:
		riskColor = color.New(color.FgYellow, color.Bold)
		riskIcon = "?"
	default: // RiskSafe
		riskColor = color.New(color.FgGreen, color.Bold)
		riskIcon = "✓"
	}

	var promptText string
	if edit {
		promptText = fmt.Sprintf("%s %s [Y/n/e]: ", riskColor.Sprint(riskIcon), prompt)
	} else {
		promptText = fmt.Sprintf("%s %s [Y/n]: ", riskColor.Sprint(riskIcon), prompt)
	}

	promptStr := promptColor.Sprint(promptText)

	confirmInput, cancelled, err := readConfirmationInput(promptStr)
	if err != nil {
		fmt.Printf("Error reading confirmation: %v\n", err)
		return false, ""
	}
	if cancelled {
		m.Status = ""
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
		defer func() { _ = os.Remove(tmpFile.Name()) }()

		// Write the command to the temporary file
		if _, err := tmpFile.WriteString(command); err != nil {
			fmt.Printf("Error writing to temporary file: %v\n", err)
			_ = tmpFile.Close()
			return false, ""
		}
		if err := tmpFile.Close(); err != nil {
			fmt.Printf("Error closing temporary file: %v\n", err)
			return false, ""
		}

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

func readConfirmationInput(prompt string) (string, bool, error) {
	fd := int(os.Stdin.Fd())

	if !term.IsTerminal(fd) {
		fmt.Print(prompt)
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", false, err
		}
		return strings.TrimRight(line, "\r\n"), false, nil
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = term.Restore(fd, oldState) }()

	fmt.Print(prompt)

	reader := bufio.NewReader(os.Stdin)
	const maxBufferSize = 4096 // Prevent unbounded memory growth
	var (
		buffer []rune
		cursor int
	)

	redraw := func() {
		fmt.Printf("\r%s%s", prompt, string(buffer))
		fmt.Print("\033[K")
		fmt.Printf("\r%s", prompt)
		if cursor > 0 {
			fmt.Print(string(buffer[:cursor]))
		}
	}

	beep := func() {
		fmt.Print("\a")
	}

	insertRune := func(r rune) {
		if len(buffer) >= maxBufferSize {
			beep()
			return
		}
		buffer = append(buffer, 0)
		copy(buffer[cursor+1:], buffer[cursor:])
		buffer[cursor] = r
		cursor++
		redraw()
	}

	for {
		r, _, err := reader.ReadRune()
		if err != nil {
			if errors.Is(err, io.EOF) {
				fmt.Print("\r\n")
				return string(buffer), false, nil
			}
			return "", false, err
		}

		switch r {
		case '\r', '\n':
			fmt.Print("\r\n")
			return string(buffer), false, nil
		case 3: // Ctrl+C
			fmt.Print("\r\n")
			return "", true, nil
		case 4: // Ctrl+D
			if len(buffer) == 0 {
				fmt.Print("\r\n")
				return "", true, nil
			}
			beep()
		case 127, 8: // Backspace/Delete
			if cursor > 0 {
				buffer = append(buffer[:cursor-1], buffer[cursor:]...)
				cursor--
				redraw()
			} else {
				beep()
			}
		case 27: // Escape sequences
			seq, seqErr := readEscapeSequence(reader, fd, 25*time.Millisecond)
			if seqErr != nil {
				return "", false, seqErr
			}
			if len(seq) == 1 {
				fmt.Print("\r\n")
				return "", true, nil
			}
			handleEscapeSequence(seq, &buffer, &cursor, redraw, beep)
		default:
			if unicode.IsPrint(r) {
				insertRune(r)
			} else {
				beep()
			}
		}
	}
}

func handleEscapeSequence(seq []byte, buffer *[]rune, cursor *int, redraw func(), beep func()) {
	if len(seq) < 2 {
		return
	}

	const (
		left  = 'D'
		right = 'C'
		up    = 'A'
		down  = 'B'
	)

	switch seq[1] {
	case '[':
		if len(seq) < 3 {
			return
		}
		switch seq[2] {
		case left:
			if *cursor > 0 {
				*cursor--
				redraw()
			} else {
				beep()
			}
		case right:
			if *cursor < len(*buffer) {
				*cursor++
				redraw()
			} else {
				beep()
			}
		case up, down:
			beep()
		case 'H':
			if *cursor != 0 {
				*cursor = 0
				redraw()
			} else {
				beep()
			}
		case 'F':
			if *cursor != len(*buffer) {
				*cursor = len(*buffer)
				redraw()
			} else {
				beep()
			}
		case '3':
			if len(seq) >= 4 && seq[3] == '~' {
				if *cursor < len(*buffer) {
					b := *buffer
					*buffer = append(b[:*cursor], b[*cursor+1:]...)
					redraw()
				} else {
					beep()
				}
			}
		}
	case 'O':
		if len(seq) < 3 {
			return
		}
		switch seq[2] {
		case 'H':
			if *cursor != 0 {
				*cursor = 0
				redraw()
			} else {
				beep()
			}
		case 'F':
			if *cursor != len(*buffer) {
				*cursor = len(*buffer)
				redraw()
			} else {
				beep()
			}
		default:
			beep()
		}
	default:
		beep()
	}
}

func readEscapeSequence(reader *bufio.Reader, fd int, timeout time.Duration) ([]byte, error) {
	seq := []byte{27}

	readNext := func() (bool, error) {
		if reader.Buffered() == 0 {
			ready, err := waitForInput(fd, timeout)
			if err != nil {
				return false, err
			}
			if !ready {
				return false, nil
			}
		}

		b, err := reader.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return false, nil
			}
			return false, err
		}
		seq = append(seq, b)
		return true, nil
	}

	if ok, err := readNext(); err != nil {
		return seq, err
	} else if !ok {
		return seq, nil
	}

	switch seq[1] {
	case '[', 'O':
		for {
			if seq[1] == '[' && len(seq) >= 3 {
				last := seq[len(seq)-1]
				if last >= 0x40 && last <= 0x7E {
					break
				}
			}
			if seq[1] == 'O' && len(seq) >= 3 {
				break
			}
			ok, err := readNext()
			if err != nil {
				return seq, err
			}
			if !ok {
				break
			}
		}
	}

	return seq, nil
}

func waitForInput(fd int, timeout time.Duration) (bool, error) {
	// Ensure minimum timeout to prevent race conditions with ESC sequences
	if timeout <= 0 {
		timeout = 10 * time.Millisecond
	}
	pollTimeout := int(timeout / time.Millisecond)
	if pollTimeout <= 0 {
		pollTimeout = 1
	}

	fds := []unix.PollFd{
		{Fd: int32(fd), Events: unix.POLLIN},
	}

	n, err := unix.Poll(fds, pollTimeout)
	if err != nil {
		if errors.Is(err, unix.EINTR) {
			return false, nil
		}
		return false, err
	}
	if n == 0 {
		return false, nil
	}
	return fds[0].Revents&unix.POLLIN != 0, nil
}
