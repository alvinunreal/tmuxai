// internal/risk_scorer.go
package internal

import (
	"fmt"
	"regexp"
	"strings"
)

type RiskLevel string

const (
	RiskSafe    RiskLevel = "safe"
	RiskUnknown RiskLevel = "unknown"
	RiskDanger  RiskLevel = "danger"
)

type RiskAssessment struct {
	Level RiskLevel
	Flags []string // Which patterns matched
}

// Pattern represents a risk detection pattern
type Pattern struct {
	Regex *regexp.Regexp
}

// CommandComponent represents a parsed command element
type CommandComponent struct {
	Command   string        // The actual command (e.g., "ls -la")
	Type      ComponentType
	Redirects []Redirect
}

type ComponentType string

const (
	SimpleCommand       ComponentType = "simple"
	PipeSource          ComponentType = "pipe_source"
	PipeDestination     ComponentType = "pipe_dest"
	CommandSubstitution ComponentType = "substitution"
)

type Redirect struct {
	Type   RedirectType // >, >>, <
	Target string       // File path or command
}

type RedirectType string

const (
	RedirectOutput RedirectType = ">"
	RedirectAppend RedirectType = ">>"
	RedirectInput  RedirectType = "<"
)

var (
	// Safe patterns - commands we explicitly trust
	safePatterns = []Pattern{
		// Basic file operations
		{regexp.MustCompile(`^ls(\s|$)`)},
		{regexp.MustCompile(`^pwd(\s|$)`)},
		{regexp.MustCompile(`^cd(\s|$)`)},
		{regexp.MustCompile(`^cat\s+[^/|><&;]`)},
		{regexp.MustCompile(`^head(\s|$)`)},
		{regexp.MustCompile(`^tail(\s|$)`)},
		{regexp.MustCompile(`^less(\s|$)`)},
		{regexp.MustCompile(`^more(\s|$)`)},
		{regexp.MustCompile(`^file(\s|$)`)},
		{regexp.MustCompile(`^stat(\s|$)`)},
		{regexp.MustCompile(`^tree(\s|$)`)},

		// Search and filter
		{regexp.MustCompile(`^grep(\s|$)`)},
		{regexp.MustCompile(`^find(\s|$)`)},
		{regexp.MustCompile(`^rg(\s|$)`)},
		{regexp.MustCompile(`^ag(\s|$)`)},
		{regexp.MustCompile(`^ack(\s|$)`)},
		{regexp.MustCompile(`^locate(\s|$)`)},

		// System info
		{regexp.MustCompile(`^which(\s|$)`)},
		{regexp.MustCompile(`^whoami(\s|$)`)},
		{regexp.MustCompile(`^date(\s|$)`)},
		{regexp.MustCompile(`^uptime(\s|$)`)},
		{regexp.MustCompile(`^uname(\s|$)`)},
		{regexp.MustCompile(`^hostname(\s|$)`)},

		// Process info (read-only)
		{regexp.MustCompile(`^ps(\s|$)`)},
		{regexp.MustCompile(`^top(\s|$)`)},
		{regexp.MustCompile(`^htop(\s|$)`)},

		// Git read operations
		{regexp.MustCompile(`^git\s+(status|log|diff|show|branch)`)},
		{regexp.MustCompile(`^git\s+ls-files`)},
		{regexp.MustCompile(`^git\s+remote`)},

		// Development tools (read-only)
		{regexp.MustCompile(`^npm\s+(list|ls|view|info)`)},
		{regexp.MustCompile(`^yarn\s+(list|info)`)},
		{regexp.MustCompile(`^go\s+(version|env|list)`)},
		{regexp.MustCompile(`^docker\s+(ps|images|inspect)`)},
		{regexp.MustCompile(`^docker\s+compose\s+(ps|config)`)},

		// Text processing
		{regexp.MustCompile(`^echo(\s|$)`)},
		{regexp.MustCompile(`^wc(\s|$)`)},
		{regexp.MustCompile(`^sort(\s|$)`)},
		{regexp.MustCompile(`^uniq(\s|$)`)},
		{regexp.MustCompile(`^cut(\s|$)`)},
		{regexp.MustCompile(`^awk(\s|$)`)},
		{regexp.MustCompile(`^sed\s+[^-]`)}, // sed without dangerous flags

		// Network utilities (read-only)
		{regexp.MustCompile(`^ping(\s|$)`)},
		{regexp.MustCompile(`^traceroute(\s|$)`)},
		{regexp.MustCompile(`^nslookup(\s|$)`)},
		{regexp.MustCompile(`^dig(\s|$)`)},
		{regexp.MustCompile(`^host(\s|$)`)},
		{regexp.MustCompile(`^curl\s+[^|]`)}, // curl without pipes
		{regexp.MustCompile(`^wget\s+[^|]`)}, // wget without pipes
		{regexp.MustCompile(`^netstat(\s|$)`)},
		{regexp.MustCompile(`^ss(\s|$)`)},
		{regexp.MustCompile(`^ifconfig(\s|$)`)},
		{regexp.MustCompile(`^ip\s+(addr|route|link)`)},

		// Disk and system utilities
		{regexp.MustCompile(`^df(\s|$)`)},
		{regexp.MustCompile(`^du(\s|$)`)},
		{regexp.MustCompile(`^free(\s|$)`)},
		{regexp.MustCompile(`^lsof(\s|$)`)},
	}

	// Dangerous patterns - major risks that require user confirmation
	dangerousPatterns = []Pattern{
		// Destructive filesystem operations (most common/dangerous)
		{regexp.MustCompile(`\brm\s+-[rR]f`)},        // rm -rf
		{regexp.MustCompile(`\brm\s+.*-[rR].*f`)},    // rm with -r and -f in any order
		{regexp.MustCompile(`\brm\s+(-[rR]\s+)?/`)},  // rm targeting root paths
		{regexp.MustCompile(`\bfind\b.*-delete\b`)},  // find with -delete flag
		// MODIFIED: Catch ANY -exec, not just -exec rm. This fixes the 'find -exec sh' bypass.
		{regexp.MustCompile(`\bfind\b.*-exec\b`)}, // find with rm execution
		{regexp.MustCompile(`\bxargs\s+rm\b`)},       // xargs with rm (mass deletion)
		{regexp.MustCompile(`\bmkfs\b`)},             // Format filesystem
		{regexp.MustCompile(`\bdd\s+.*of=/dev/`)},    // Write to device
		{regexp.MustCompile(`\bfdisk\b`)},            // Partition management
		{regexp.MustCompile(`\bparted\b`)},           // Partition editor
		{regexp.MustCompile(`:\s*,\s*\$\s*d\b`)},     // dd in sed (delete all lines)
		{regexp.MustCompile(`\btruncate\s+-s\s*0`)},  // Truncate files to zero size

		// Privilege escalation (very common)
		{regexp.MustCompile(`\bsudo\b`)},
		{regexp.MustCompile(`\bsu\s`)},
		{regexp.MustCompile(`\bdoas\b`)}, // OpenBSD sudo alternative

		// Dangerous permissions
		{regexp.MustCompile(`\bchmod\s+[0-7]*[67][0-7]*\b`)}, // chmod with exec bits
		{regexp.MustCompile(`\bchmod\s+777`)},                // chmod 777 (world writable)
		{regexp.MustCompile(`\bchown\s+.*root`)},             // chown to root

		// Code execution risks
		{regexp.MustCompile(`\|\s*(sh|bash|zsh|fish)\b`)}, // pipe to shell
		{regexp.MustCompile(`\beval\s`)},                   // eval command
		{regexp.MustCompile(`\bexec\s`)},                   // exec command
		{regexp.MustCompile(`\bcurl\b.*\|\s*(sh|bash)`)},   // curl | sh
		{regexp.MustCompile(`\bwget\b.*\|\s*(sh|bash)`)},   // wget | sh
		{regexp.MustCompile(`\bsource\s+/dev/(tcp|udp)`)},  // network file execution
		{regexp.MustCompile(`\.\s+/dev/(tcp|udp)`)},        // dot source network
		{regexp.MustCompile(`\bperl\s+-e`)},                // perl one-liner execution
		{regexp.MustCompile(`\bpython\s+-c`)},              // python one-liner execution
		{regexp.MustCompile(`\bruby\s+-e`)},                // ruby one-liner execution
		{regexp.MustCompile(`\bawk\s+.*system\(`)},         // awk with system() calls
		{regexp.MustCompile(`\b:\(\)\s*\{.*:\|:`)},         // fork bomb pattern

		// System critical modifications
		// REMOVED: Redundant, covered by new `[<>]` rule
		// {regexp.MustCompile(`>\s*/etc/`)},                                 // Writing to system config
		{regexp.MustCompile(`\b(systemctl|service)\s+(stop|disable|mask)`)}, // Stop/disable services
		{regexp.MustCompile(`\breboot\b`)},                                // Restart system
		{regexp.MustCompile(`\bshutdown\b`)},                              // Shutdown system
		{regexp.MustCompile(`\bhalt\b`)},                                  // Halt system
		{regexp.MustCompile(`\bpoweroff\b`)},                              // Power off system
		{regexp.MustCompile(`\bkillall\b`)},                               // Kill all processes by name
		{regexp.MustCompile(`\bpkill\b`)},                                 // Kill processes by pattern
		{regexp.MustCompile(`\bkill\s+-9`)},                               // Force kill signal
		{regexp.MustCompile(`\binit\s+[016]`)},                            // Change runlevel

		// Package management (can install/remove critical packages)
		{regexp.MustCompile(`\bapt(-get)?\s+(remove|purge|autoremove)`)}, // apt remove
		{regexp.MustCompile(`\byum\s+(remove|erase)`)},                   // yum remove
		{regexp.MustCompile(`\bdnf\s+(remove|erase)`)},                   // dnf remove
		{regexp.MustCompile(`\bpacman\s+-R`)},                            // pacman remove
		{regexp.MustCompile(`\bbrew\s+(uninstall|remove)`)},              // brew remove
		{regexp.MustCompile(`\bnpm\s+(uninstall|remove)\s+-g`)},          // npm global uninstall

		// Disk/filesystem operations
		{regexp.MustCompile(`\bumount\s+/`)},     // Unmount root paths
		{regexp.MustCompile(`\bfsck\b`)},         // Filesystem check (can modify)
		{regexp.MustCompile(`\bmount\s+.*-o.*rw`)}, // Remount with write

		// Database operations
		{regexp.MustCompile(`\b(mysql|psql|mongo).*drop\s+(database|table)`)}, // Drop database/table
		{regexp.MustCompile(`\bDROP\s+(DATABASE|TABLE)\b`)},                   // SQL DROP

		// Docker/Container dangerous ops
		{regexp.MustCompile(`\bdocker\s+(rm|rmi)\s+.*-f`)},           // Force remove
		{regexp.MustCompile(`\bdocker\s+system\s+prune\s+.*-a`)},     // Remove all unused
		{regexp.MustCompile(`\bkubectl\s+delete`)},                   // Kubernetes delete
		{regexp.MustCompile(`\bdocker\s+compose\s+down\s+.*-v`)},     // Remove volumes

		// Git dangerous operations
		{regexp.MustCompile(`\bgit\s+push\s+.*--force`)},        // Force push
		{regexp.MustCompile(`\bgit\s+clean\s+.*-[fFdDxX]`)},     // Clean untracked files
		{regexp.MustCompile(`\bgit\s+reset\s+.*--hard`)},        // Hard reset
		{regexp.MustCompile(`\bgit\s+branch\s+.*-D`)},           // Force delete branch

		// Cron/scheduled tasks
		{regexp.MustCompile(`\bcrontab\s+-r`)}, // Remove all cron jobs

	}
)

// ParseCommand splits a command string into components
func ParseCommand(cmd string) ([]CommandComponent, error) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return []CommandComponent{}, nil
	}

	// Split by multiple operators: ; && || &
	// For now, we'll do a simple approach that handles the test cases
	components := []CommandComponent{}
	
	// Split by semicolon first
	semicolonParts := strings.Split(cmd, ";")
	for _, semicolonPart := range semicolonParts {
		semicolonPart = strings.TrimSpace(semicolonPart)
		if semicolonPart == "" {
			continue
		}
		
		// Split by && 
		andParts := strings.Split(semicolonPart, "&&")
		for _, andPart := range andParts {
			andPart = strings.TrimSpace(andPart)
			if andPart == "" {
				continue
			}
			
			// Split by ||
			orParts := strings.Split(andPart, "||")
			for _, orPart := range orParts {
				orPart = strings.TrimSpace(orPart)
				if orPart == "" {
					continue
				}
				
				// Check for redirects and command substitution
				component := CommandComponent{
					Command: orPart,
					Type:    SimpleCommand,
				}
				
				// Check for redirects (both dangerous and unknown)
				if strings.Contains(orPart, " > ") {
					// Extract redirect target
					parts := strings.Split(orPart, " > ")
					if len(parts) > 1 {
						target := strings.TrimSpace(parts[1])
						// Take first word as the target (ignore additional arguments)
						targetParts := strings.Fields(target)
						if len(targetParts) > 0 {
							component.Redirects = append(component.Redirects, Redirect{
								Type:   RedirectOutput,
								Target: targetParts[0],
							})
						}
					}
				}
				if strings.Contains(orPart, " >> ") {
					// Extract redirect target
					parts := strings.Split(orPart, " >> ")
					if len(parts) > 1 {
						target := strings.TrimSpace(parts[1])
						targetParts := strings.Fields(target)
						if len(targetParts) > 0 {
							component.Redirects = append(component.Redirects, Redirect{
								Type:   RedirectAppend,
								Target: targetParts[0],
							})
						}
					}
				}
				
				// Check for command substitution
				if strings.Contains(orPart, "$(") {
					// Extract the substituted command for separate evaluation
					start := strings.Index(orPart, "$(")
					if start != -1 {
						end := strings.Index(orPart[start:], ")")
						if end != -1 {
							substitutedCmd := orPart[start+2 : start+end]
							components = append(components, CommandComponent{
								Command: substitutedCmd,
								Type:    CommandSubstitution,
							})
						}
					}
				}
				
				components = append(components, component)
			}
		}
	}
	
	return components, nil
}

// evaluateComponent assesses risk for a single command component
func evaluateComponent(component CommandComponent) RiskAssessment {
	assessment := RiskAssessment{
		Level: RiskUnknown, // Default to unknown
		Flags: []string{},
	}

	cmd := strings.TrimSpace(component.Command)
	if cmd == "" {
		assessment.Level = RiskSafe
		return assessment
	}

	// Check for dangerous patterns first (highest priority)
	for _, pattern := range dangerousPatterns {
		if pattern.Regex.MatchString(cmd) {
			assessment.Level = RiskDanger
			assessment.Flags = append(assessment.Flags, pattern.Regex.String())
		}
	}

	// If dangerous patterns found, return immediately
	if assessment.Level == RiskDanger {
		return assessment
	}

	// Check for safe patterns
	for _, pattern := range safePatterns {
		if pattern.Regex.MatchString(cmd) {
			assessment.Level = RiskSafe
			return assessment
		}
	}

	// If no matches, it's unknown (requires user confirmation)
	return assessment
}

// checkRedirect evaluates if a redirect operation is dangerous
func checkRedirect(redirect Redirect) RiskLevel {
	dangerousRedirectPaths := []string{
		"/etc/",
		"/sys/",
		"/proc/",
		"/dev/",
		"/boot/",
		"/root/",
	}
	
	if redirect.Type == RedirectOutput || redirect.Type == RedirectAppend {
		// Check for dangerous system paths
		for _, dangerousPath := range dangerousRedirectPaths {
			if strings.HasPrefix(redirect.Target, dangerousPath) {
				return RiskDanger
			}
		}
		
		// Check for command substitution in redirect target
		if strings.Contains(redirect.Target, "$(") {
			return RiskUnknown
		}
		
		// Other redirects to user paths are unknown (could overwrite important files)
		return RiskUnknown
	}
	
	return RiskSafe
}

// aggregateRisks combines multiple assessments using maximum risk level
func aggregateRisks(assessments []RiskAssessment) RiskAssessment {
	if len(assessments) == 0 {
		return RiskAssessment{Level: RiskSafe, Flags: []string{}}
	}

	result := RiskAssessment{
		Level: RiskSafe,
		Flags: []string{},
	}

	// Find the maximum risk level and combine all flags
	for _, assessment := range assessments {
		// Combine flags
		result.Flags = append(result.Flags, assessment.Flags...)
		
		// Update to maximum risk level
		if assessment.Level == RiskDanger {
			result.Level = RiskDanger
		} else if assessment.Level == RiskUnknown && result.Level != RiskDanger {
			result.Level = RiskUnknown
		}
	}

	return result
}

func ScoreCommand(cmd string) RiskAssessment {
	// 1. Parse command into components
	components, err := ParseCommand(cmd)
	if err != nil {
		// On parse error, treat as unknown for safety
		return RiskAssessment{Level: RiskUnknown, Flags: []string{"parse_error"}}
	}

	// Handle empty command
	if len(components) == 0 {
		return RiskAssessment{Level: RiskSafe, Flags: []string{}}
	}

	// 2. Evaluate each component
	assessments := []RiskAssessment{}
	for i, component := range components {
		assessment := evaluateComponent(component)
		// Add component index to flags for debugging
		for j, flag := range assessment.Flags {
			assessment.Flags[j] = fmt.Sprintf("component_%d_%s", i, flag)
		}
		assessments = append(assessments, assessment)
		
		// Check redirects
		for _, redirect := range component.Redirects {
			redirectRisk := checkRedirect(redirect)
			if redirectRisk != RiskSafe {
				assessments = append(assessments, RiskAssessment{
					Level: redirectRisk,
					Flags: []string{fmt.Sprintf("redirect_%s_%s", redirect.Type, redirect.Target)},
				})
			}
		}
	}

	// 3. Aggregate results
	return aggregateRisks(assessments)
}
