package model

import (
	"testing"
)

func TestCategorizeExt(t *testing.T) {
	cases := []struct {
		name     string
		filename string
		want     string
	}{
		{"jpg image", "photo.JPG", "图片"},
		{"png image", "a.png", "图片"},
		{"mp4 video", "movie.mp4", "视频"},
		{"mp3 audio", "song.mp3", "音频"},
		{"pdf doc", "report.pdf", "文档"},
		{"zip archive", "bundle.zip", "压缩包"},
		{"iso image", "ubuntu.iso", "程序镜像"},
		{"unknown ext", "data.xyz", "其他"},
		{"no ext", "README", "其他"},
		{"dotfile", ".gitignore", "其他"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := categorizeExt(tc.filename); got != tc.want {
				t.Errorf("categorizeExt(%q) = %q, want %q", tc.filename, got, tc.want)
			}
		})
	}
}

func TestMapToSlice(t *testing.T) {
	in := map[string]int64{"图片": 10, "视频": 30, "文档": 30}
	got := mapToSlice(in)
	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(got))
	}
	// Sorted by value desc; ties broken by name asc ("文档" < "视频").
	if got[0].Value != 30 || got[0].Name != "文档" {
		t.Errorf("first entry = %+v, want {文档 30}", got[0])
	}
	if got[1].Value != 30 || got[1].Name != "视频" {
		t.Errorf("second entry = %+v, want {视频 30}", got[1])
	}
	if got[2].Name != "图片" {
		t.Errorf("last entry = %+v, want 图片", got[2])
	}
}

func TestMapToSliceEmpty(t *testing.T) {
	got := mapToSlice(map[string]int64{})
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d entries", len(got))
	}
}

func TestTopByDownloads(t *testing.T) {
	files := []*File{
		{Filename: "a.txt", DownloadCounter: 0},
		{Filename: "b.txt", DownloadCounter: 5},
		{Filename: "c.txt", DownloadCounter: 3},
	}
	got := topByDownloads(files)
	if len(got) != 2 {
		t.Fatalf("expected 2 entries (zero-download excluded), got %d", len(got))
	}
	if got[0].Filename != "b.txt" || got[1].Filename != "c.txt" {
		t.Errorf("order = [%s, %s], want [b.txt, c.txt]", got[0].Filename, got[1].Filename)
	}
}

func TestTopByDownloadsCap(t *testing.T) {
	files := make([]*File, 0, 15)
	for i := 0; i < 15; i++ {
		files = append(files, &File{Filename: "f", DownloadCounter: i + 1})
	}
	if got := topByDownloads(files); len(got) != topListSize {
		t.Errorf("expected cap at %d, got %d", topListSize, len(got))
	}
}

func TestTopBySize(t *testing.T) {
	files := []*File{
		{Filename: "a", Size: 0},
		{Filename: "b", Size: 100},
		{Filename: "c", Size: 50},
	}
	got := topBySize(files)
	if len(got) != 2 {
		t.Fatalf("expected 2 entries (zero-size excluded), got %d", len(got))
	}
	if got[0].Filename != "b" || got[1].Filename != "c" {
		t.Errorf("order = [%s, %s], want [b, c]", got[0].Filename, got[1].Filename)
	}
}
