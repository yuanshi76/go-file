package common

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// WebDAVClient downloads objects from a WebDAV endpoint that fronts the same
// storage as the OSS bucket. Downloads use the (free) WebDAV channel instead of
// paid OSS egress.
type WebDAVClient struct {
	http     *http.Client
	base     *url.URL
	username string
	password string
	root     string
}

// NewWebDAVClient builds a client from the current archive configuration and the
// env-loaded password.
func NewWebDAVClient() (*WebDAVClient, error) {
	if !WebDAVConfigReady() {
		return nil, fmt.Errorf("WebDAV 配置不完整：请检查 Base URL/用户名/密码")
	}
	base := strings.TrimRight(strings.TrimSpace(WebDAVBaseURL), "/")
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		base = "http://" + base
	}
	u, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("WebDAV Base URL 无效，请填写完整 http(s) URL: %w", err)
	}
	return &WebDAVClient{
		http:     &http.Client{Timeout: 0},
		base:     u,
		username: strings.TrimSpace(WebDAVUsername),
		password: WebDAVPassword(),
		root:     strings.Trim(WebDAVRootPrefix, "/"),
	}, nil
}

// urlForKey maps a relative key to its full WebDAV URL, applying the configured
// root prefix.
func (c *WebDAVClient) urlForKey(key string) string {
	mapped := JoinKey(c.root, key)
	u := *c.base
	path := strings.TrimRight(u.Path, "/")
	u.Path = path + "/" + mapped
	u.RawPath = path + "/" + percentPath(mapped)
	return u.String()
}

// HeadSize returns the object's size as reported by WebDAV.
func (c *WebDAVClient) HeadSize(key string) (int64, error) {
	req, err := http.NewRequest(http.MethodHead, c.urlForKey(key), nil)
	if err != nil {
		return 0, fmt.Errorf("failed to build WebDAV head request: %w", err)
	}
	req.SetBasicAuth(c.username, c.password)
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("WebDAV head error: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return 0, fmt.Errorf("WebDAV head failed: %d", resp.StatusCode)
	}
	return strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
}

// Download streams the object at key to destPath, creating parent directories as
// needed. It writes to a temporary file first and renames on success so a failed
// transfer never leaves a truncated file in place. When expectedSize > 0 the
// downloaded size is verified.
func (c *WebDAVClient) Download(key, destPath string, expectedSize int64) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0750); err != nil {
		return fmt.Errorf("failed to create directory for %s: %w", destPath, err)
	}
	req, err := http.NewRequest(http.MethodGet, c.urlForKey(key), nil)
	if err != nil {
		return fmt.Errorf("failed to build WebDAV get request: %w", err)
	}
	req.SetBasicAuth(c.username, c.password)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("WebDAV download error: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("WebDAV download failed: %d", resp.StatusCode)
	}

	tmp := fmt.Sprintf("%s.tmp-%d", destPath, time.Now().UnixNano())
	out, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	written, err := io.Copy(out, resp.Body)
	closeErr := out.Close()
	if err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("failed to write download: %w", err)
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("failed to flush download: %w", closeErr)
	}
	if expectedSize > 0 && written != expectedSize {
		_ = os.Remove(tmp)
		return fmt.Errorf("WebDAV download size mismatch: got %d want %d", written, expectedSize)
	}
	if err := os.Rename(tmp, destPath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("failed to finalize download: %w", err)
	}
	return nil
}
