package model

import (
	"path"
	"sort"
	"strings"
)

// NameValue is a generic chart-friendly pair used across the stats dashboard.
// The JSON tags match what ECharts series expect ({name, value}).
type NameValue struct {
	Name  string `json:"name"`
	Value int64  `json:"value"`
}

// FileEntry is a slim projection of a File used in "top" lists, exposing only
// the fields the dashboard renders.
type FileEntry struct {
	Filename        string `json:"filename"`
	Link            string `json:"link"`
	Uploader        string `json:"uploader"`
	Time            string `json:"time"`
	Size            int64  `json:"size"`
	DownloadCounter int    `json:"download_counter"`
	Category        string `json:"category"`
	Archived        bool   `json:"archived"`
}

// FileStats is the full payload powering the manage-page statistics dashboard.
// Everything here is derived from a single scan over the files table, so it
// works whether or not Redis is configured.
type FileStats struct {
	TotalFiles     int64       `json:"total_files"`
	TotalSize      int64       `json:"total_size"`
	TotalDownloads int64       `json:"total_downloads"`
	ArchivedFiles  int64       `json:"archived_files"`
	LocalFiles     int64       `json:"local_files"`
	TypeSize       []NameValue `json:"type_size"`     // bytes per category
	TypeCount      []NameValue `json:"type_count"`    // file count per category
	StorageState   []NameValue `json:"storage_state"` // local vs archived counts
	TopDownloads   []FileEntry `json:"top_downloads"` // most downloaded files
	LargestFiles   []FileEntry `json:"largest_files"` // biggest files on record
	UploadTrend    []NameValue `json:"upload_trend"`  // uploads per month (YYYY-MM)
}

const topListSize = 10

// categorizeExt buckets a filename by extension into a human-friendly category.
func categorizeExt(filename string) string {
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(filename), "."))
	switch ext {
	case "jpg", "jpeg", "png", "gif", "bmp", "webp", "svg", "ico", "tiff", "heic":
		return "图片"
	case "mp4", "mkv", "avi", "mov", "wmv", "flv", "webm", "m4v", "mpeg", "rmvb":
		return "视频"
	case "mp3", "wav", "flac", "aac", "ogg", "m4a", "wma", "ape":
		return "音频"
	case "pdf", "doc", "docx", "xls", "xlsx", "ppt", "pptx", "txt", "md", "csv", "epub":
		return "文档"
	case "zip", "rar", "7z", "tar", "gz", "bz2", "xz", "zst":
		return "压缩包"
	case "iso", "img", "exe", "msi", "dmg", "apk", "deb", "rpm", "bin":
		return "程序镜像"
	default:
		return "其他"
	}
}

// mapToSlice converts a category→amount map into a slice sorted by value desc,
// giving charts a stable, meaningful order.
func mapToSlice(m map[string]int64) []NameValue {
	out := make([]NameValue, 0, len(m))
	for k, v := range m {
		out = append(out, NameValue{Name: k, Value: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Value != out[j].Value {
			return out[i].Value > out[j].Value
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// toEntry projects a File into the slim dashboard form.
func toEntry(f *File) FileEntry {
	return FileEntry{
		Filename:        f.Filename,
		Link:            f.Link,
		Uploader:        f.Uploader,
		Time:            f.Time,
		Size:            f.Size,
		DownloadCounter: f.DownloadCounter,
		Category:        categorizeExt(f.Filename),
		Archived:        f.IsArchived(),
	}
}

// ComputeFileStats scans every file row once and aggregates all dashboard
// metrics. It deliberately avoids Redis so the dashboard is available in any
// deployment.
func ComputeFileStats() (*FileStats, error) {
	files, err := AllFiles()
	if err != nil {
		return nil, err
	}

	stats := &FileStats{}
	sizeByType := make(map[string]int64)
	countByType := make(map[string]int64)
	trend := make(map[string]int64)

	for _, f := range files {
		stats.TotalFiles++
		stats.TotalSize += f.Size
		stats.TotalDownloads += int64(f.DownloadCounter)
		if f.IsArchived() {
			stats.ArchivedFiles++
		} else {
			stats.LocalFiles++
		}

		cat := categorizeExt(f.Filename)
		sizeByType[cat] += f.Size
		countByType[cat]++

		// Time format is "2006-01-02 15:04:05"; bucket uploads by month.
		if len(f.Time) >= 7 {
			trend[f.Time[:7]]++
		}
	}

	stats.TypeSize = mapToSlice(sizeByType)
	stats.TypeCount = mapToSlice(countByType)
	stats.StorageState = []NameValue{
		{Name: "本地存储", Value: stats.LocalFiles},
		{Name: "冷存储", Value: stats.ArchivedFiles},
	}

	// Upload trend is chronological so the line chart reads left-to-right.
	trendSlice := make([]NameValue, 0, len(trend))
	for k, v := range trend {
		trendSlice = append(trendSlice, NameValue{Name: k, Value: v})
	}
	sort.Slice(trendSlice, func(i, j int) bool {
		return trendSlice[i].Name < trendSlice[j].Name
	})
	stats.UploadTrend = trendSlice

	stats.TopDownloads = topByDownloads(files)
	stats.LargestFiles = topBySize(files)

	return stats, nil
}

// topByDownloads returns the most-downloaded files (download count > 0) capped
// at topListSize.
func topByDownloads(files []*File) []FileEntry {
	sorted := make([]*File, len(files))
	copy(sorted, files)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].DownloadCounter > sorted[j].DownloadCounter
	})
	out := make([]FileEntry, 0, topListSize)
	for _, f := range sorted {
		if f.DownloadCounter <= 0 || len(out) >= topListSize {
			break
		}
		out = append(out, toEntry(f))
	}
	return out
}

// topBySize returns the largest files capped at topListSize.
func topBySize(files []*File) []FileEntry {
	sorted := make([]*File, len(files))
	copy(sorted, files)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Size > sorted[j].Size
	})
	out := make([]FileEntry, 0, topListSize)
	for _, f := range sorted {
		if f.Size <= 0 || len(out) >= topListSize {
			break
		}
		out = append(out, toEntry(f))
	}
	return out
}
