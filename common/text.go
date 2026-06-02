package common

import "strings"

// SanitizeText removes angle brackets from user-supplied display text as a
// defense-in-depth measure against stored XSS. Templates rendered via
// html/template already escape output, but stripping the characters keeps the
// stored value safe even if it is ever rendered in a raw context.
func SanitizeText(s string) string {
	replacer := strings.NewReplacer("<", "", ">", "")
	return replacer.Replace(s)
}
