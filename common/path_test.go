package common

import (
	"path/filepath"
	"testing"
)

func TestIsSubPath(t *testing.T) {
	root := filepath.FromSlash("/data/upload")
	tests := []struct {
		name   string
		target string
		want   bool
	}{
		{"same as root", "/data/upload", true},
		{"direct child", "/data/upload/file.txt", true},
		{"nested child", "/data/upload/2026-06/a.bin", true},
		{"parent escape", "/data/upload/../secret", false},
		{"traversal in middle", "/data/upload/../../etc/passwd", false},
		{"sibling prefix bypass", "/data/upload-secret/file", false},
		{"unrelated path", "/etc/passwd", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSubPath(root, filepath.FromSlash(tt.target))
			if got != tt.want {
				t.Errorf("IsSubPath(%q, %q) = %v, want %v", root, tt.target, got, tt.want)
			}
		})
	}
}

func TestSanitizeText(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"plain", "plain"},
		{"<script>alert(1)</script>", "scriptalert(1)/script"},
		{"a<b>c", "abc"},
		{"no brackets here", "no brackets here"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := SanitizeText(tt.in); got != tt.want {
			t.Errorf("SanitizeText(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestIsTrustedOrigin(t *testing.T) {
	const host = "files.example.com"
	tests := []struct {
		name    string
		origin  string
		referer string
		want    bool
	}{
		{"no headers (non-browser client)", "", "", true},
		{"matching origin", "https://files.example.com", "", true},
		{"matching origin with port path ignored", "http://files.example.com", "", true},
		{"cross-site origin", "https://evil.com", "", false},
		{"origin empty, matching referer", "", "https://files.example.com/login", true},
		{"origin empty, cross-site referer", "", "https://evil.com/x", false},
		{"origin wins over referer", "https://evil.com", "https://files.example.com/x", false},
		{"malformed origin", "::not a url::", "", false},
		{"origin without host", "/relative/only", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTrustedOrigin(tt.origin, tt.referer, host); got != tt.want {
				t.Errorf("IsTrustedOrigin(%q, %q, %q) = %v, want %v", tt.origin, tt.referer, host, got, tt.want)
			}
		})
	}
}
