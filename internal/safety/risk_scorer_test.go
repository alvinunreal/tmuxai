// internal/safety/risk_scorer_test.go
package safety

import (
	"strings"
	"testing"
)

func TestScoreCommand_HighRisk(t *testing.T) {
	cmd := "rm -rf ./* ./.??*"
	assessment := ScoreCommand(cmd)
	if assessment.Level != RiskHigh {
		t.Fatalf(
			"expected risk level %s, got %s (reasons: %v)",
			RiskHigh,
			assessment.Level,
			assessment.Reasons,
		)
	}
}

func TestScoreCommand_MediumRisk(t *testing.T) {
	cmd := "mv important_file /tmp"
	assessment := ScoreCommand(cmd)
	if assessment.Level != RiskMedium {
		t.Fatalf(
			"expected risk level %s, got %s",
			RiskMedium,
			assessment.Level,
		)
	}
}

func TestScoreCommand_Safe(t *testing.T) {
	cmd := "ls -la /home"
	assessment := ScoreCommand(cmd)
	if assessment.Level != RiskSafe {
		t.Fatalf(
			"expected risk level %s, got %s",
			RiskSafe,
			assessment.Level,
		)
	}
}

func TestScoreCommand_Empty(t *testing.T) {
	cmd := ""
	assessment := ScoreCommand(cmd)
	if assessment.Level != RiskSafe {
		t.Fatalf(
			"expected risk level %s for empty command, got %s",
			RiskSafe,
			assessment.Level,
		)
	}
	if len(assessment.Reasons) != 0 {
		t.Fatalf(
			"expected no reasons for empty command, got %v",
			assessment.Reasons,
		)
	}
	if len(assessment.Flags) != 0 {
		t.Fatalf(
			"expected no flags for empty command, got %v",
			assessment.Flags,
		)
	}
}

func TestScoreCommand_OnlyWhitespace(t *testing.T) {
	cmd := "   \t  "
	assessment := ScoreCommand(cmd)
	if assessment.Level != RiskSafe {
		t.Fatalf(
			"expected risk level %s for whitespace-only command, got %s",
			RiskSafe,
			assessment.Level,
		)
	}
}

func TestScoreCommand_VeryLongCommand(t *testing.T) {
	// Create a very long safe command
	cmd := "echo " + strings.Repeat("hello ", 1000)
	assessment := ScoreCommand(cmd)
	if assessment.Level != RiskSafe {
		t.Fatalf(
			"expected risk level %s for long command, got %s",
			RiskSafe,
			assessment.Level,
		)
	}
}

func TestScoreCommand_OverlappingPatterns(t *testing.T) {
	// This command matches both "sudo" (high) and "chmod" (medium)
	// Should pick the highest risk level
	cmd := "sudo chmod 777 /etc/passwd"
	assessment := ScoreCommand(cmd)
	if assessment.Level != RiskHigh {
		t.Fatalf(
			"expected risk level %s for overlapping patterns, got %s",
			RiskHigh,
			assessment.Level,
		)
	}
	if len(assessment.Reasons) < 2 {
		t.Fatalf(
			"expected at least 2 reasons (sudo + chmod), got %d: %v",
			len(assessment.Reasons),
			assessment.Reasons,
		)
	}
	if len(assessment.Flags) < 2 {
		t.Fatalf(
			"expected at least 2 flags, got %d: %v",
			len(assessment.Flags),
			assessment.Flags,
		)
	}
}

// High-risk command tests
func TestScoreCommand_RmRfPattern(t *testing.T) {
	cmd := "rm -rf /var/www/*"
	assessment := ScoreCommand(cmd)
	if assessment.Level != RiskHigh {
		t.Fatalf(
			"expected risk level %s for rm -rf, got %s",
			RiskHigh,
			assessment.Level,
		)
	}
}

func TestScoreCommand_CurlCommand(t *testing.T) {
	cmd := "curl https://example.com/script.sh"
	assessment := ScoreCommand(cmd)
	if assessment.Level != RiskHigh {
		t.Fatalf(
			"expected risk level %s for curl, got %s",
			RiskHigh,
			assessment.Level,
		)
	}
}

func TestScoreCommand_CurlPipe(t *testing.T) {
	cmd := "curl https://example.com/script.sh | sh"
	assessment := ScoreCommand(cmd)
	if assessment.Level != RiskHigh {
		t.Fatalf(
			"expected risk level %s for curl pipe, got %s",
			RiskHigh,
			assessment.Level,
		)
	}
	// Should detect both patterns
	if len(assessment.Flags) < 2 {
		t.Fatalf(
			"expected multiple flags for curl pipe, got %d",
			len(assessment.Flags),
		)
	}
}

func TestScoreCommand_PipeToSh(t *testing.T) {
	cmd := "cat config.sh | sh"
	assessment := ScoreCommand(cmd)
	if assessment.Level != RiskHigh {
		t.Fatalf(
			"expected risk level %s for pipe to sh, got %s",
			RiskHigh,
			assessment.Level,
		)
	}
}

func TestScoreCommand_PipeToBash(t *testing.T) {
	cmd := "python script.py | bash"
	assessment := ScoreCommand(cmd)
	if assessment.Level != RiskHigh {
		t.Fatalf(
			"expected risk level %s for pipe to bash, got %s",
			RiskHigh,
			assessment.Level,
		)
	}
}

func TestScoreCommand_EvalCommand(t *testing.T) {
	cmd := "eval $(cat untrusted_file)"
	assessment := ScoreCommand(cmd)
	if assessment.Level != RiskHigh {
		t.Fatalf(
			"expected risk level %s for eval, got %s",
			RiskHigh,
			assessment.Level,
		)
	}
}

func TestScoreCommand_ExecCommand(t *testing.T) {
	cmd := "exec rm -rf /"
	assessment := ScoreCommand(cmd)
	if assessment.Level != RiskHigh {
		t.Fatalf(
			"expected risk level %s for exec, got %s",
			RiskHigh,
			assessment.Level,
		)
	}
}

func TestScoreCommand_DdCommand(t *testing.T) {
	cmd := "dd if=/dev/zero of=/dev/sda bs=1M"
	assessment := ScoreCommand(cmd)
	if assessment.Level != RiskHigh {
		t.Fatalf(
			"expected risk level %s for dd command, got %s",
			RiskHigh,
			assessment.Level,
		)
	}
}

func TestScoreCommand_MkfsCommand(t *testing.T) {
	cmd := "mkfs.ext4 /dev/sda1"
	assessment := ScoreCommand(cmd)
	if assessment.Level != RiskHigh {
		t.Fatalf(
			"expected risk level %s for mkfs command, got %s",
			RiskHigh,
			assessment.Level,
		)
	}
}

func TestScoreCommand_SudoCommand(t *testing.T) {
	cmd := "sudo systemctl restart networking"
	assessment := ScoreCommand(cmd)
	if assessment.Level != RiskHigh {
		t.Fatalf(
			"expected risk level %s for sudo, got %s",
			RiskHigh,
			assessment.Level,
		)
	}
}

// Medium-risk command tests
func TestScoreCommand_ChmodCommand(t *testing.T) {
	cmd := "chmod 755 myfile.txt"
	assessment := ScoreCommand(cmd)
	if assessment.Level != RiskMedium {
		t.Fatalf(
			"expected risk level %s for chmod, got %s",
			RiskMedium,
			assessment.Level,
		)
	}
}

func TestScoreCommand_Chmod777(t *testing.T) {
	cmd := "chmod 777 /tmp/shared"
	assessment := ScoreCommand(cmd)
	if assessment.Level != RiskMedium {
		t.Fatalf(
			"expected risk level %s for chmod 777, got %s",
			RiskMedium,
			assessment.Level,
		)
	}
}

func TestScoreCommand_ChownMedium(t *testing.T) {
	cmd := "chown root:root /var/www"
	assessment := ScoreCommand(cmd)
	if assessment.Level != RiskMedium {
		t.Fatalf(
			"expected risk level %s for chown, got %s",
			RiskMedium,
			assessment.Level,
		)
	}
}

func TestScoreCommand_RmPattern(t *testing.T) {
	cmd := "rm important.txt"
	assessment := ScoreCommand(cmd)
	if assessment.Level != RiskMedium {
		t.Fatalf(
			"expected risk level %s for rm, got %s",
			RiskMedium,
			assessment.Level,
		)
	}
}

func TestScoreCommand_MvPattern(t *testing.T) {
	cmd := "mv /etc/config /tmp"
	assessment := ScoreCommand(cmd)
	if assessment.Level != RiskMedium {
		t.Fatalf(
			"expected risk level %s for mv, got %s",
			RiskMedium,
			assessment.Level,
		)
	}
}

func TestScoreCommand_SedInPlace(t *testing.T) {
	cmd := "sed -i 's/old/new/g' config.txt"
	assessment := ScoreCommand(cmd)
	if assessment.Level != RiskMedium {
		t.Fatalf(
			"expected risk level %s for sed -i, got %s",
			RiskMedium,
			assessment.Level,
		)
	}
}

func TestScoreCommand_TeeCommand(t *testing.T) {
	cmd := "cat logfile | tee /var/log/output"
	assessment := ScoreCommand(cmd)
	if assessment.Level != RiskMedium {
		t.Fatalf(
			"expected risk level %s for tee, got %s",
			RiskMedium,
			assessment.Level,
		)
	}
}

// Safe command tests (table-driven)
func TestScoreCommand_SafeCommonCommands(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
	}{
		{"ls long listing", "ls -la /home"},
		{"cat file", "cat /etc/hostname"},
		{"grep search", "grep root /etc/passwd"},
		{"echo output", "echo hello world"},
		{"pwd directory", "pwd"},
		{"find search", "find /home -name '*.txt'"},
		{"git status", "git status"},
		{"docker ps", "docker ps -a"},
		{"ps aux", "ps aux | grep nginx"},
		{"head file", "head -20 logfile.txt"},
		{"tail file", "tail -f logfile.txt"},
		{"wc count", "wc -l file.txt"},
		{"sort", "sort data.txt"},
		{"uniq", "uniq -c data.txt"},
		{"awk", "awk '{print $1}' data.txt"},
		{"date", "date"},
		{"whoami", "whoami"},
		{"hostname", "hostname"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assessment := ScoreCommand(tt.cmd)
			if assessment.Level != RiskSafe {
				t.Fatalf(
					"expected risk level %s, got %s (reasons: %v)",
					RiskSafe,
					assessment.Level,
					assessment.Reasons,
				)
			}
		})
	}
}

func TestScoreCommand_RiskLevelRanking(t *testing.T) {
	// Verify that higher risk patterns override lower ones
	tests := []struct {
		name        string
		cmd         string
		expectedMin RiskLevel
	}{
		{"high only", "rm -rf /", RiskHigh},
		{"medium only", "chmod 755 file", RiskMedium},
		{"high + medium", "sudo rm important", RiskHigh},
		{"safe", "echo hello", RiskSafe},
		{"multiple medium", "mv file && chown user file", RiskMedium},
		{"eval high", "eval malicious_code", RiskHigh},
		{"pipe to sh high", "cat script | sh", RiskHigh},
		{"exec high", "exec command", RiskHigh},
		{"sed -i medium", "sed -i 's/x/y/' file", RiskMedium},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assessment := ScoreCommand(tt.cmd)
			if assessment.Level != tt.expectedMin {
				t.Fatalf(
					"expected risk level %s, got %s",
					tt.expectedMin,
					assessment.Level,
				)
			}
		})
	}
}

func TestScoreCommand_AssessmentStructure(t *testing.T) {
	// Verify that the assessment structure is properly populated
	cmd := "sudo rm -rf /home"
	assessment := ScoreCommand(cmd)

	if assessment.Level == "" {
		t.Fatal("expected risk level to be set")
	}

	if len(assessment.Reasons) == 0 {
		t.Fatal("expected reasons to be populated")
	}

	if len(assessment.Flags) == 0 {
		t.Fatal("expected flags to be populated")
	}

	// Verify reasons contain human-readable text
	for _, reason := range assessment.Reasons {
		if len(reason) == 0 {
			t.Fatal("expected non-empty reason string")
		}
	}

	// Verify flags contain the matched patterns
	for _, flag := range assessment.Flags {
		if len(flag) == 0 {
			t.Fatal("expected non-empty flag string")
		}
	}
}

func TestScoreCommand_CaseSensitivity(t *testing.T) {
	// Note: current implementation is case-sensitive
	// This test documents that behavior
	tests := []struct {
		name     string
		cmd      string
		expected RiskLevel
	}{
		{"lowercase rm", "rm file.txt", RiskMedium},
		{"uppercase RM", "RM file.txt", RiskSafe}, // Should not match
		{"lowercase sudo", "sudo apt-get install", RiskHigh},
		{"uppercase SUDO", "SUDO reboot", RiskSafe}, // Should not match
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assessment := ScoreCommand(tt.cmd)
			if assessment.Level != tt.expected {
				t.Fatalf(
					"expected risk level %s, got %s",
					tt.expected,
					assessment.Level,
				)
			}
		})
	}
}

func TestScoreCommand_ComplexPipelines(t *testing.T) {
	// Test complex command pipelines
	tests := []struct {
		name     string
		cmd      string
		expected RiskLevel
	}{
		{
			"pipe chain to shell",
			"cat config | grep setting | sh",
			RiskHigh,
		},
		{
			"curl to eval",
			"curl https://example.com/setup.sh | eval",
			RiskHigh,
		},
		{
			"safe pipe",
			"cat file | grep pattern | sort",
			RiskSafe,
		},
		{
			"sed in pipe",
			"cat data | sed -i 's/x/y/' > output",
			RiskMedium,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assessment := ScoreCommand(tt.cmd)
			if assessment.Level != tt.expected {
				t.Fatalf(
					"expected risk level %s, got %s (reasons: %v)",
					tt.expected,
					assessment.Level,
					assessment.Reasons,
				)
			}
		})
	}
}

func TestScoreCommand_AllPatternsCovered(t *testing.T) {
	// Ensure all documented patterns are tested
	patterns := []struct {
		pattern string
		level   RiskLevel
	}{
		{"rm -rf", RiskHigh},
		{"sudo", RiskHigh},
		{"mkfs", RiskHigh},
		{"dd if=", RiskHigh},
		{"curl", RiskHigh},
		{"curl | sh", RiskHigh},
		{"| sh", RiskHigh},
		{"| bash", RiskHigh},
		{"eval ", RiskHigh},
		{"exec ", RiskHigh},
		{"chmod", RiskMedium},
		{"rm ", RiskMedium},
		{"mv ", RiskMedium},
		{"chown", RiskMedium},
		{"sed -i", RiskMedium},
		{"tee ", RiskMedium},
	}

	for _, p := range patterns {
		t.Run("pattern: "+p.pattern, func(t *testing.T) {
			assessment := ScoreCommand(p.pattern)
			if assessment.Level != p.level {
				t.Fatalf(
					"pattern '%s': expected %s, got %s",
					p.pattern,
					p.level,
					assessment.Level,
				)
			}
		})
	}
}
