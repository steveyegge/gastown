package cmd

import (
	"fmt"
	"regexp"
	"strings"
)

// credentialPattern describes a credential detection pattern.
type credentialPattern struct {
	name    string
	pattern *regexp.Regexp
}

// credentialPatterns lists patterns that indicate credentials in message text.
var credentialPatterns = []credentialPattern{
	{
		name:    "Brazilian CPF (11-digit number)",
		pattern: regexp.MustCompile(`\b\d{3}\.?\d{3}\.?\d{3}-?\d{2}\b`),
	},
	{
		name:    "password/senha field with value",
		pattern: regexp.MustCompile(`(?i)\b(senha|password|passwd|pwd)\s*[:=]\s*\S+`),
	},
	{
		name:    "AWS access key",
		pattern: regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	},
	{
		name:    "AWS secret key",
		pattern: regexp.MustCompile(`(?i)\baws[_\-.]?secret[_\-.]?access[_\-.]?key\s*[:=]\s*\S+`),
	},
	{
		name:    "API token / secret field with value",
		pattern: regexp.MustCompile(`(?i)\b(api[_\-.]?key|api[_\-.]?token|access[_\-.]?token|secret[_\-.]?key|auth[_\-.]?token|bearer)\s*[:=]\s*\S{8,}`),
	},
	{
		name:    "GitHub personal access token",
		pattern: regexp.MustCompile(`\bghp_[A-Za-z0-9]{36}\b|\bgho_[A-Za-z0-9]{36}\b|\bgh[srua]_[A-Za-z0-9]{36,76}\b`),
	},
	{
		name:    "private key header",
		pattern: regexp.MustCompile(`-----BEGIN (?:RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----`),
	},
}

// credentialScanResult holds a detected credential match.
type credentialScanResult struct {
	patternName string
	snippet     string // short redacted snippet for display
}

// scanForCredentials checks text for potential credential patterns.
// Returns a list of findings (empty if none found).
func scanForCredentials(text string) []credentialScanResult {
	var findings []credentialScanResult
	for _, cp := range credentialPatterns {
		if loc := cp.pattern.FindStringIndex(text); loc != nil {
			// Build a short redacted snippet for display
			raw := text[loc[0]:loc[1]]
			snippet := redactSnippet(raw)
			findings = append(findings, credentialScanResult{
				patternName: cp.name,
				snippet:     snippet,
			})
		}
	}
	return findings
}

// redactSnippet redacts the latter portion of a match for display.
func redactSnippet(s string) string {
	if len(s) <= 8 {
		return strings.Repeat("*", len(s))
	}
	visible := s[:4]
	return visible + strings.Repeat("*", len(s)-4)
}

// warnCredentials prints a warning about detected credentials and returns an
// error if the user hasn't passed --allow-credentials.
func warnCredentials(findings []credentialScanResult, allowCredentials bool) error {
	if len(findings) == 0 {
		return nil
	}

	fmt.Println()
	fmt.Println("⚠️  CREDENTIAL WARNING: potential secrets detected in message body:")
	for _, f := range findings {
		fmt.Printf("   • %s (e.g. %q)\n", f.patternName, f.snippet)
	}
	fmt.Println()
	fmt.Println("   Sending credentials via gt mail creates a PERMANENT Dolt record.")
	fmt.Println("   Instead: write secrets to a gitignored .env file and share the path.")
	fmt.Println()

	if !allowCredentials {
		return fmt.Errorf(
			"message blocked: credential patterns detected (%d match(es))\n"+
				"  To override: gt mail send --allow-credentials ...\n"+
				"  To fix properly: use a .env file instead of inline credentials",
			len(findings),
		)
	}

	fmt.Println("   --allow-credentials passed — sending anyway (credentials will be permanently stored)")
	fmt.Println()
	return nil
}
