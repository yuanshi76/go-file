package controller

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go-file/common"
	"go-file/model"
)

// The AI API exposes a small, machine-friendly surface so an AI agent can
// discover what files exist and download them autonomously, using the same
// per-user token (Authorization header) as the rest of the API. Responses use
// the project-wide {success, message, data} envelope, and the projection
// deliberately omits internal fields (storage link, OSS key) while adding
// AI-convenient derived fields (human-readable size, category, ready-to-fetch
// download URL).

const (
	aiDefaultPageSize = 20
	aiMaxPageSize     = 100
)

// AIFile is the machine-friendly projection of a file returned by the AI API.
type AIFile struct {
	Id              int    `json:"id"`
	Filename        string `json:"filename"`
	Description     string `json:"description"`
	Uploader        string `json:"uploader"`
	Time            string `json:"time"`
	Size            int64  `json:"size"`
	SizeHuman       string `json:"size_human"`
	Category        string `json:"category"`
	DownloadCounter int    `json:"download_counter"`
	Archived        bool   `json:"archived"`
	DownloadURL     string `json:"download_url"`
}

// humanizeBytes renders a byte count as a human-readable size (binary units).
func humanizeBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// clampAtoi parses s as an int, falling back to def on empty/invalid input and
// clamping the result into [min, max]. Used to harden pagination query params.
func clampAtoi(s string, def, min, max int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}

// parseLenientID extracts a positive integer file id from the sloppy values a
// weak model tends to emit: quoted ("7"), spaced (" 7 "), or float-ish ("7.0").
// Returns ok=false when there is no usable id.
func parseLenientID(s string) (int, bool) {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "\"'")
	s = strings.TrimSpace(s)
	if dot := strings.IndexByte(s, '.'); dot >= 0 {
		s = s[:dot] // tolerate "7.0"
	}
	if s == "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// rankByName orders candidates so the best textual match to query comes first:
// exact filename (case-insensitive) > prefix > contains > the rest, breaking
// ties by newest (highest id). This makes name-based lookups behave intuitively
// for an AI that only knows a rough filename.
func rankByName(files []*model.File, query string) {
	q := strings.ToLower(strings.TrimSpace(query))
	score := func(f *model.File) int {
		name := strings.ToLower(f.Filename)
		switch {
		case name == q:
			return 3
		case strings.HasPrefix(name, q):
			return 2
		case strings.Contains(name, q):
			return 1
		default:
			return 0
		}
	}
	sort.SliceStable(files, func(i, j int) bool {
		si, sj := score(files[i]), score(files[j])
		if si != sj {
			return si > sj
		}
		return files[i].Id > files[j].Id
	})
}

// resolveFile finds a single file from a loose query that may be a numeric id or
// a (possibly partial) filename. Returns the best match and the ranked candidate
// list (for ambiguity hints), or nil when nothing matches. This single helper is
// what lets a weak model download by name in one step.
func resolveFile(query string) (*model.File, []*model.File, error) {
	if id, ok := parseLenientID(query); ok {
		f, err := model.FileById(id)
		if err != nil {
			return nil, nil, err
		}
		if f != nil {
			return f, []*model.File{f}, nil
		}
		// Not a real id — fall through and try it as a filename.
	}
	files, _, err := model.ListFilesPaged(strings.TrimSpace(query), 0, 20)
	if err != nil {
		return nil, nil, err
	}
	if len(files) == 0 {
		return nil, nil, nil
	}
	rankByName(files, query)
	return files[0], files, nil
}

// requestBaseURL reconstructs the externally-visible base URL of the request so
// download links are absolute and directly usable by an AI agent. It honors a
// reverse proxy's X-Forwarded-Proto when present.
func requestBaseURL(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	if proto := c.GetHeader("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}
	return scheme + "://" + c.Request.Host
}

// toAIFile projects a stored file into its AI-facing form.
func toAIFile(c *gin.Context, f *model.File) AIFile {
	return AIFile{
		Id:              f.Id,
		Filename:        f.Filename,
		Description:     f.Description,
		Uploader:        f.Uploader,
		Time:            f.Time,
		Size:            f.Size,
		SizeHuman:       humanizeBytes(f.Size),
		Category:        model.FileCategory(f.Filename),
		DownloadCounter: f.DownloadCounter,
		Archived:        f.IsArchived(),
		DownloadURL:     fmt.Sprintf("%s/api/ai/files/%d/content", requestBaseURL(c), f.Id),
	}
}

// AIListFiles returns a paginated, optionally-searched list of files as JSON,
// letting an AI agent discover what is available for download.
func AIListFiles(c *gin.Context) {
	page := clampAtoi(c.Query("page"), 1, 1, 1<<30)
	pageSize := clampAtoi(c.Query("page_size"), aiDefaultPageSize, 1, aiMaxPageSize)
	query := strings.TrimSpace(c.Query("q"))

	files, total, err := model.ListFilesPaged(query, (page-1)*pageSize, pageSize)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error(), "data": nil})
		return
	}
	items := make([]AIFile, 0, len(files))
	for _, f := range files {
		items = append(items, toAIFile(c, f))
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"items":     items,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// AIGetFile returns metadata for a single file addressed by id.
func AIGetFile(c *gin.Context) {
	id, ok := parseLenientID(c.Param("id"))
	if !ok {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的文件 id；若只知道文件名，请改用 GET /api/ai/find?q=<文件名>", "data": nil})
		return
	}
	f, err := model.FileById(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error(), "data": nil})
		return
	}
	if f == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": fmt.Sprintf("未找到 id=%d 的文件；可用 GET /api/ai/find?q= 按名称查找。", id), "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": toAIFile(c, f)})
}

// serveFileBytes streams a resolved file to the client, transparently restoring
// it from cold storage first when archived. Shared by the id- and query-based
// download handlers.
func serveFileBytes(c *gin.Context, f *model.File) {
	fullPath := filepath.Join(common.UploadPath, f.Link)
	if !common.IsSubPath(common.UploadPath, fullPath) {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "非法的文件路径", "data": nil})
		return
	}
	if f.IsArchived() {
		restored, rerr := EnsureLocalCopy(f)
		if rerr != nil {
			common.SysError("AI download restore " + f.Link + " failed: " + rerr.Error())
			c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": "文件已归档至云端，取回失败，请稍后重试", "data": nil})
			return
		}
		fullPath = restored
	}
	c.FileAttachment(fullPath, f.Filename)
	go model.UpdateDownloadCounter(f.Link)
}

// AIDownloadFile streams the raw bytes of a file addressed by id. Archived files
// are transparently restored from cold storage first.
func AIDownloadFile(c *gin.Context) {
	id, ok := parseLenientID(c.Param("id"))
	if !ok {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的文件 id；若只知道文件名，请改用 GET /api/ai/download?q=<文件名>", "data": nil})
		return
	}
	f, err := model.FileById(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error(), "data": nil})
		return
	}
	if f == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": fmt.Sprintf("未找到 id=%d 的文件；可用 GET /api/ai/download?q=<文件名> 按名称下载。", id), "data": nil})
		return
	}
	serveFileBytes(c, f)
}

// findSummary builds a short natural-language hint so a weak model can decide
// the next step without parsing the structured list.
func findSummary(query string, items []AIFile) string {
	if len(items) == 0 {
		return fmt.Sprintf("未找到与 \"%s\" 匹配的文件，请换个关键词再试。", query)
	}
	best := items[0]
	if len(items) == 1 {
		return fmt.Sprintf("找到 1 个文件：%s（id=%d，%s）。可调用 download_file 用 q=\"%s\" 或 id=%d 下载。",
			best.Filename, best.Id, best.SizeHuman, best.Filename, best.Id)
	}
	return fmt.Sprintf("找到 %d 个文件，最匹配的是 %s（id=%d）。可调用 download_file 用 q=\"%s\" 下载，或细化关键词再查。",
		len(items), best.Filename, best.Id, best.Filename)
}

// AIFindFiles resolves a loose query (filename or keyword) to the best-matching
// files, returning a compact, ranked list tuned for weak models: few items, a
// ready id/download_url, and a natural-language summary to act on next.
func AIFindFiles(c *gin.Context) {
	query := strings.TrimSpace(c.Query("q"))
	if query == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "请提供 q 参数（文件名或关键词），例如 q=报告", "data": nil})
		return
	}
	limit := clampAtoi(c.Query("limit"), 5, 1, 20)
	_, candidates, err := resolveFile(query)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error(), "data": nil})
		return
	}
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	items := make([]AIFile, 0, len(candidates))
	for _, f := range candidates {
		items = append(items, toAIFile(c, f))
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"items":   items,
			"count":   len(items),
			"summary": findSummary(query, items),
		},
	})
}

// AIDownloadByQuery streams the best match for a loose query (id or filename) so
// a weak model can download in a single step without first tracking a numeric id.
func AIDownloadByQuery(c *gin.Context) {
	query := strings.TrimSpace(c.Query("q"))
	if query == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "请提供 q 参数（文件 id 或文件名），例如 q=report.pdf", "data": nil})
		return
	}
	f, _, err := resolveFile(query)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error(), "data": nil})
		return
	}
	if f == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": fmt.Sprintf("未找到与 \"%s\" 匹配的文件，请先用 find_files 搜索确认文件名。", query), "data": nil})
		return
	}
	serveFileBytes(c, f)
}

// AIUploadFile lets an AI agent push one or more files back into the store via a
// multipart POST (field "file", optional "description"), returning the created
// file records as JSON. Files land on local storage and are indistinguishable
// from web uploads. Size is bounded by the UploadSizeLimit middleware and the
// configured per-file limit.
func AIUploadFile(c *gin.Context) {
	uploader := c.GetString("username")
	if uploader == "" {
		uploader = "AI"
	}
	description := strings.TrimSpace(c.PostForm("description"))
	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "解析表单失败: " + err.Error(), "data": nil})
		return
	}
	// Be forgiving about the field name: weak models and varying MCP clients send
	// "file", "files", "file[]", "upload", etc. Fall back to any provided file.
	files := form.File["file"]
	if len(files) == 0 {
		for _, alt := range []string{"files", "file[]", "upload", "data"} {
			if fs := form.File[alt]; len(fs) > 0 {
				files = fs
				break
			}
		}
	}
	if len(files) == 0 {
		for _, fs := range form.File {
			if len(fs) > 0 {
				files = fs
				break
			}
		}
	}
	if len(files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "未提供文件，请使用 multipart/form-data，字段名 file 携带文件", "data": nil})
		return
	}
	currentTime := time.Now().Format("2006-01-02 15:04:05")
	created := make([]AIFile, 0, len(files))
	for _, fh := range files {
		if limit := common.MaxUploadBytes(); limit > 0 && fh.Size > limit {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"success": false,
				"message": fmt.Sprintf("文件 %s 超过上传大小限制（%d MB）", fh.Filename, common.MaxUploadSizeMB),
				"data":    nil,
			})
			return
		}
		link, savePath, serr := storeUploadedFile(c, common.UploadPath, fh)
		if serr != nil {
			common.SysError("AI upload save failed: " + serr.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": serr.Error(), "data": nil})
			return
		}
		var size int64
		if fi, statErr := os.Stat(savePath); statErr == nil {
			size = fi.Size()
		}
		fileObj := &model.File{
			Description:  description,
			Uploader:     uploader,
			Time:         currentTime,
			Link:         link,
			Filename:     filepath.Base(fh.Filename),
			Size:         size,
			StorageState: common.StorageLocal,
		}
		if err := fileObj.Insert(); err != nil {
			common.SysError("AI upload insert failed: " + err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "写入数据库失败: " + err.Error(), "data": nil})
			return
		}
		created = append(created, toAIFile(c, fileObj))
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    gin.H{"items": created, "count": len(created)},
	})
}

// AIStats returns the file-centric statistics payload (reusing the dashboard
// computation) so an AI agent can summarize the store at a glance.
func AIStats(c *gin.Context) {
	stats, err := model.ComputeFileStats()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error(), "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": stats})
}

// AIManifest is a public, self-describing document so an AI agent can learn the
// API shape and auth scheme without out-of-band documentation. It contains no
// secrets and no per-instance data beyond the public base URL.
func AIManifest(c *gin.Context) {
	base := requestBaseURL(c)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"name":        "go-file AI API",
			"version":     "v1",
			"description": "只读文件发现与下载接口，供 AI 代理获取文件列表/元数据/统计并自主下载文件。",
			"base_url":    base,
			"auth": gin.H{
				"type":    "bearer_token",
				"header":  "Authorization",
				"example": "Authorization: <token>",
				"note":    "在用户设置页生成 token，所有 /api/ai/* 接口（manifest 除外）均需携带。",
			},
			"endpoints": []gin.H{
				{"method": "GET", "path": "/api/ai/manifest", "auth": false, "summary": "本说明文档（无需鉴权）"},
				{"method": "GET", "path": "/api/ai/find", "auth": true, "summary": "按文件名/关键词找文件，返回最匹配项与自然语言提示（推荐弱模型先用它）",
					"query": gin.H{"q": "文件名或关键词（必填）", "limit": "返回条数，默认 5，最大 20"}},
				{"method": "GET", "path": "/api/ai/download", "auth": true, "summary": "按 id 或文件名一步下载（推荐弱模型用它，无需先记 id）",
					"query": gin.H{"q": "文件 id 或文件名（必填）"}},
				{"method": "GET", "path": "/api/ai/files", "auth": true, "summary": "列出/搜索文件（分页）",
					"query": gin.H{"q": "关键词，匹配文件名/描述/上传者/时间", "page": "页码，默认 1", "page_size": "每页数量，默认 20，最大 100"}},
				{"method": "POST", "path": "/api/ai/files", "auth": true, "summary": "上传文件（multipart，字段 file 可多个，可选 description）",
					"body": gin.H{"file": "二进制文件（multipart/form-data，可重复）", "description": "可选，文件描述"}},
				{"method": "GET", "path": "/api/ai/files/:id", "auth": true, "summary": "获取单个文件元数据"},
				{"method": "GET", "path": "/api/ai/files/:id/content", "auth": true, "summary": "下载文件二进制内容（归档文件自动从冷存储取回）"},
				{"method": "GET", "path": "/api/ai/stats", "auth": true, "summary": "文件维度统计（总数/占用/下载/分类等）"},
			},
		},
	})
}
