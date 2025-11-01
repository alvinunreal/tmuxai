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
