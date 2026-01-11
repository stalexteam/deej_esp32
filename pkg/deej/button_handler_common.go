package deej

import "strings"

// processEscapeSequences processes escape sequences in text
// Converts \n, \t, \r, \\ to actual characters
// This function is shared between Windows and Linux implementations
func processEscapeSequences(text string) string {
	// Process escape sequences:
	// \n = newline
	// \t = tab
	// \r = carriage return
	// \\ = backslash

	// First handle \\ to avoid double processing
	// Use a temporary marker that won't appear in normal text
	result := strings.ReplaceAll(text, "\\\\", "\x00")

	// Convert escape sequences
	result = strings.ReplaceAll(result, "\\n", "\n")
	result = strings.ReplaceAll(result, "\\r", "\r")
	result = strings.ReplaceAll(result, "\\t", "\t")

	// Restore backslashes
	result = strings.ReplaceAll(result, "\x00", "\\")

	return result
}
