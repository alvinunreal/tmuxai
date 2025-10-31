// internal/safety/risk_scorer.go
package safety

import "strings"

type RiskLevel string

const (
	RiskSafe   RiskLevel = "safe"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

type RiskAssessment struct {
	Level   RiskLevel
	Reasons []string
	Flags   []string // Which parts are risky
}

func ScoreCommand(cmd string) RiskAssessment {
	var assessment RiskAssessment
	assessment.Level = RiskSafe

	// Helper to rank risk levels (higher number = higher risk)
	rank := func(r RiskLevel) int {
		switch r {
		case RiskHigh:
			return 3
		case RiskMedium:
			return 2
		case RiskSafe:
			return 1
		default:
			return 0
		}
	}

	// Dangerous patterns
	// Updated to cover additional edge cases:
	//   - Any use of `curl` is considered high risk (covers pipe to sh)
	//   - Any use of `chmod` is considered medium risk (covers variations like 755)
	dangerousPatterns := map[string]RiskLevel{
		"rm -rf":    RiskHigh,   // recursive remove
		"sudo":      RiskHigh,   // root privileges
		"mkfs":      RiskHigh,   // make filesystem
		"dd if=":    RiskHigh,   // byte copying
		"curl":      RiskHigh,   // matches any curl command, including pipe to sh
		"curl | sh": RiskHigh,   // retained for explicit pipe detection
		"| sh":      RiskHigh,   // pipe to shell
		"| bash":    RiskHigh,   // pipe to bash
		"eval ":     RiskHigh,   // code evaluation
		"exec ":     RiskHigh,   // process execution
		"chmod":     RiskMedium, // matches any chmod command (e.g., 777, 755)
		"rm ":       RiskMedium, // non-recursive remove
		"mv ":       RiskMedium, // moving files
		"chown":     RiskMedium, // changing ownership
		"sed -i":    RiskMedium, // in-place editing
		"tee ":      RiskMedium, // write to multiple outputs
	}

	for pattern, risk := range dangerousPatterns {
		if strings.Contains(cmd, pattern) {
			// Upgrade risk level only if this pattern is higher than the current level
			if rank(risk) > rank(assessment.Level) {
				assessment.Level = risk
			}
			assessment.Reasons = append(assessment.Reasons, "Contains: "+pattern)
			assessment.Flags = append(assessment.Flags, pattern)
		}
	}

	return assessment
}
