package css

import "strings"

// SanitizeIdentifier converts a string to a valid CSS identifier
// by replacing spaces and special characters with valid alternatives.
//
// Examples:
//   - "C++" -> "Cplusplus"
//   - "C#" -> "Csharp"
//   - "DIGITAL Command Language" -> "DIGITAL-Command-Language"
//   - "Objective-C++" -> "Objective-Cplusplus"
func SanitizeIdentifier(name string) string {
	// Replace spaces and special characters with hyphens
	replacer := strings.NewReplacer(
		" ", "-",
		"+", "plus",
		"#", "sharp",
	)
	sanitized := replacer.Replace(name)

	// Remove any remaining invalid characters
	sanitized = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return -1
	}, sanitized)

	return sanitized
}
