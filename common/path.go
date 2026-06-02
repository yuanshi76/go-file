package common

import (
	"path/filepath"
	"strings"
)

// IsSubPath reports whether target resides within root (or equals root).
// It defends against path traversal by resolving the relative path instead of
// relying on a string prefix check, which is vulnerable to sibling-directory
// bypasses such as "/data" matching "/data-secret".
func IsSubPath(root string, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}
