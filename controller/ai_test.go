package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go-file/model"
)

func TestHumanizeBytes(t *testing.T) {
	cases := []struct {
		name string
		in   int64
		want string
	}{
		{"zero", 0, "0 B"},
		{"bytes", 512, "512 B"},
		{"exactly 1KiB", 1024, "1.00 KiB"},
		{"kib", 1536, "1.50 KiB"},
		{"mib", 5 * 1024 * 1024, "5.00 MiB"},
		{"gib", 3 * 1024 * 1024 * 1024, "3.00 GiB"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := humanizeBytes(tc.in); got != tc.want {
				t.Errorf("humanizeBytes(%d) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestClampAtoi(t *testing.T) {
	cases := []struct {
		name          string
		s             string
		def, min, max int
		want          int
	}{
		{"empty falls back to default", "", 20, 1, 100, 20},
		{"invalid falls back to default", "abc", 20, 1, 100, 20},
		{"in range", "42", 20, 1, 100, 42},
		{"below min clamps up", "-5", 20, 1, 100, 1},
		{"above max clamps down", "999", 20, 1, 100, 100},
		{"at boundaries", "1", 20, 1, 100, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := clampAtoi(tc.s, tc.def, tc.min, tc.max); got != tc.want {
				t.Errorf("clampAtoi(%q,%d,%d,%d) = %d, want %d", tc.s, tc.def, tc.min, tc.max, got, tc.want)
			}
		})
	}
}

func TestParseLenientID(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		wantN  int
		wantOk bool
	}{
		{"plain", "7", 7, true},
		{"spaced", "  7 ", 7, true},
		{"quoted", "\"7\"", 7, true},
		{"single quoted", "'42'", 42, true},
		{"float-ish", "7.0", 7, true},
		{"empty", "", 0, false},
		{"zero", "0", 0, false},
		{"negative", "-3", 0, false},
		{"name", "report.pdf", 0, false},
		{"letters", "abc", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n, ok := parseLenientID(tc.in)
			if n != tc.wantN || ok != tc.wantOk {
				t.Errorf("parseLenientID(%q) = (%d,%v), want (%d,%v)", tc.in, n, ok, tc.wantN, tc.wantOk)
			}
		})
	}
}

func TestRankByName(t *testing.T) {
	files := []*model.File{
		{Id: 1, Filename: "annual-report.pdf"},
		{Id: 2, Filename: "report.pdf"},
		{Id: 5, Filename: "report.pdf"}, // newer exact match, should beat id 2
		{Id: 3, Filename: "report-draft.docx"},
		{Id: 4, Filename: "unrelated.txt"},
	}
	rankByName(files, "report.pdf")

	// Exact match, newest first.
	if files[0].Id != 5 {
		t.Errorf("expected exact newest (id=5) first, got id=%d", files[0].Id)
	}
	if files[1].Id != 2 {
		t.Errorf("expected exact older (id=2) second, got id=%d", files[1].Id)
	}
	// A "contains" match must outrank a non-match: annual-report.pdf (contains
	// "report.pdf") should come before unrelated.txt (no match).
	pos := func(name string) int {
		for i, f := range files {
			if f.Filename == name {
				return i
			}
		}
		return -1
	}
	if pos("annual-report.pdf") > pos("unrelated.txt") {
		t.Errorf("contains-match should outrank non-match: annual-report.pdf at %d, unrelated.txt at %d",
			pos("annual-report.pdf"), pos("unrelated.txt"))
	}
}

func newTestContext(t *testing.T, target string, headers map[string]string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", target, nil)
	for k, v := range headers {
		c.Request.Header.Set(k, v)
	}
	return c
}

func TestRequestBaseURL(t *testing.T) {
	cases := []struct {
		name    string
		target  string
		headers map[string]string
		want    string
	}{
		{"plain http", "http://example.com/api/ai/files", nil, "http://example.com"},
		{"forwarded proto wins", "http://example.com/api/ai/files",
			map[string]string{"X-Forwarded-Proto": "https"}, "https://example.com"},
		{"host with port", "http://example.com:3000/api/ai/files", nil, "http://example.com:3000"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestContext(t, tc.target, tc.headers)
			if got := requestBaseURL(c); got != tc.want {
				t.Errorf("requestBaseURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestToAIFile(t *testing.T) {
	c := newTestContext(t, "http://example.com/api/ai/files", nil)
	f := &model.File{
		Id:              7,
		Filename:        "clip.mp4",
		Description:     "demo",
		Uploader:        "alice",
		Time:            "2026-06-23 10:00:00",
		Size:            5 * 1024 * 1024,
		DownloadCounter: 3,
		StorageState:    "archived",
	}
	got := toAIFile(c, f)

	if got.Id != 7 || got.Filename != "clip.mp4" {
		t.Errorf("identity fields not copied: %+v", got)
	}
	if got.Category != "视频" {
		t.Errorf("Category = %q, want 视频", got.Category)
	}
	if got.SizeHuman != "5.00 MiB" {
		t.Errorf("SizeHuman = %q, want 5.00 MiB", got.SizeHuman)
	}
	if !got.Archived {
		t.Errorf("Archived = false, want true for archived storage state")
	}
	if got.DownloadURL != "http://example.com/api/ai/files/7/content" {
		t.Errorf("DownloadURL = %q, unexpected", got.DownloadURL)
	}
}
