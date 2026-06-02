package controller

import "testing"

func TestSafeRedirectTarget(t *testing.T) {
	tests := []struct {
		name    string
		referer string
		want    string
	}{
		{"empty referer", "", "/"},
		{"same-site relative", "https://example.com/explorer", "/explorer"},
		{"same-site with query", "https://example.com/explorer?p=a%2Fb", "/explorer?p=a%2Fb"},
		{"login page falls back", "https://example.com/login", "/"},
		{"protocol-relative open redirect", "https://example.com//evil.com", "/"},
		{"root", "https://example.com/", "/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := safeRedirectTarget(tt.referer); got != tt.want {
				t.Errorf("safeRedirectTarget(%q) = %q, want %q", tt.referer, got, tt.want)
			}
		})
	}
}
