package common

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// OSSClient is a minimal Aliyun OSS client that signs requests with the V4
// (OSS4-HMAC-SHA256) scheme. It implements only the operations the archival
// pipeline needs: streaming PUT, HEAD and DELETE. The signing logic is ported
// from the AsymDrive reference project.
type OSSClient struct {
	http          *http.Client
	bucket        string
	region        string
	accessKeyID   string
	secret        string
	securityToken string
	endpoint      *url.URL // bucket-scoped endpoint, e.g. https://bucket.oss-cn-beijing.aliyuncs.com
}

// NewOSSClient builds a client from the current archive configuration and the
// env-loaded secret. It returns an error when the configuration is incomplete.
func NewOSSClient() (*OSSClient, error) {
	if !OSSConfigReady() {
		return nil, fmt.Errorf("OSS 配置不完整：请检查 bucket/endpoint/region/AccessKey")
	}
	endpoint, err := ossBucketEndpoint(OSSEndpoint, OSSBucket)
	if err != nil {
		return nil, err
	}
	return &OSSClient{
		http:          &http.Client{Timeout: 0},
		bucket:        strings.TrimSpace(OSSBucket),
		region:        strings.TrimSpace(OSSRegion),
		accessKeyID:   strings.TrimSpace(OSSAccessKeyID),
		secret:        OSSAccessKeySecret(),
		securityToken: OSSSecurityToken(),
		endpoint:      endpoint,
	}, nil
}

// PutObjectFromFile streams the file at filePath to OSS under key. The body is
// sent with UNSIGNED-PAYLOAD so it is never buffered into memory.
func (c *OSSClient) PutObjectFromFile(key, filePath string) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("failed to stat %s: %w", filePath, err)
	}
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", filePath, err)
	}
	defer f.Close()

	header := http.Header{}
	header.Set("Content-Type", "application/octet-stream")
	resp, err := c.do(http.MethodPut, key, nil, header, f, info.Size())
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("OSS upload failed %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// HeadObject returns the object's size and whether it exists.
func (c *OSSClient) HeadObject(key string) (size int64, exists bool, err error) {
	resp, err := c.do(http.MethodHead, key, nil, http.Header{}, nil, 0)
	if err != nil {
		return 0, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return 0, false, nil
	}
	if resp.StatusCode/100 != 2 {
		return 0, false, fmt.Errorf("OSS head failed: %d", resp.StatusCode)
	}
	n, _ := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	return n, true, nil
}

// DeleteObject removes the object. A missing object is treated as success.
func (c *OSSClient) DeleteObject(key string) error {
	resp, err := c.do(http.MethodDelete, key, nil, http.Header{}, nil, 0)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("OSS delete failed: %d", resp.StatusCode)
	}
	return nil
}

// do builds, signs and sends a single OSS request.
func (c *OSSClient) do(method, key string, query map[string]string, header http.Header, body io.Reader, contentLength int64) (*http.Response, error) {
	u := c.urlFor(key, query)
	now := time.Now().UTC()
	dateGMT := now.Format("Mon, 02 Jan 2006 15:04:05 GMT")
	ossDate := now.Format("20060102T150405Z")
	signDate := now.Format("20060102")

	if header == nil {
		header = http.Header{}
	}
	header.Set("Date", dateGMT)
	header.Set("x-oss-date", ossDate)
	header.Set("x-oss-content-sha256", "UNSIGNED-PAYLOAD")
	if c.securityToken != "" {
		header.Set("x-oss-security-token", c.securityToken)
	}
	host := u.Host
	header.Set("Host", host)

	contentType := header.Get("Content-Type")
	signature, additional := c.v4Signature(method, c.canonicalURI(key), query, header, contentType, ossDate, signDate)
	var auth string
	if additional == "" {
		auth = fmt.Sprintf("OSS4-HMAC-SHA256 Credential=%s/%s/%s/oss/aliyun_v4_request,Signature=%s",
			c.accessKeyID, signDate, c.region, signature)
	} else {
		auth = fmt.Sprintf("OSS4-HMAC-SHA256 Credential=%s/%s/%s/oss/aliyun_v4_request,AdditionalHeaders=%s,Signature=%s",
			c.accessKeyID, signDate, c.region, additional, signature)
	}
	header.Set("Authorization", auth)

	req, err := http.NewRequest(method, u.String(), body)
	if err != nil {
		return nil, fmt.Errorf("failed to build OSS request: %w", err)
	}
	req.Header = header
	req.Host = host
	if body != nil {
		req.ContentLength = contentLength
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OSS request error: %w", err)
	}
	return resp, nil
}

// v4Signature computes the OSS4-HMAC-SHA256 signature and the AdditionalHeaders
// list, mirroring the canonical-request construction used by AsymDrive.
func (c *OSSClient) v4Signature(method, canonicalURI string, query map[string]string, header http.Header, contentType, ossDate, signDate string) (signature, additional string) {
	canonicalHeaders := map[string]string{
		"x-oss-content-sha256": "UNSIGNED-PAYLOAD",
		"x-oss-date":           ossDate,
	}
	if contentType != "" {
		canonicalHeaders["content-type"] = contentType
	}
	for name, values := range header {
		lower := strings.ToLower(name)
		if lower == "host" || strings.HasPrefix(lower, "x-oss-") {
			if len(values) > 0 {
				canonicalHeaders[lower] = strings.TrimSpace(values[0])
			}
		}
	}

	names := make([]string, 0, len(canonicalHeaders))
	for name := range canonicalHeaders {
		names = append(names, name)
	}
	sort.Strings(names)

	var headerText strings.Builder
	required := map[string]bool{"content-type": true, "x-oss-content-sha256": true, "x-oss-date": true}
	var additionalNames []string
	for _, name := range names {
		headerText.WriteString(name)
		headerText.WriteString(":")
		headerText.WriteString(canonicalHeaders[name])
		headerText.WriteString("\n")
		if !required[name] {
			additionalNames = append(additionalNames, name)
		}
	}
	additional = strings.Join(additionalNames, ";")

	canonicalRequest := strings.Join([]string{
		method,
		canonicalURI,
		canonicalQuery(query),
		headerText.String(),
		additional,
		"UNSIGNED-PAYLOAD",
	}, "\n")
	canonicalHash := sha256Hex(canonicalRequest)
	scope := fmt.Sprintf("%s/%s/oss/aliyun_v4_request", signDate, c.region)
	stringToSign := strings.Join([]string{"OSS4-HMAC-SHA256", ossDate, scope, canonicalHash}, "\n")
	signingKey := v4SigningKey(c.secret, signDate, c.region)
	return hex.EncodeToString(hmacSHA256(signingKey, stringToSign)), additional
}

func (c *OSSClient) urlFor(key string, query map[string]string) *url.URL {
	u := *c.endpoint
	u.Path = "/" + percentPath(key)
	u.RawPath = u.Path
	if len(query) > 0 {
		u.RawQuery = canonicalQuery(query)
	}
	return &u
}

func (c *OSSClient) canonicalURI(key string) string {
	if key == "" {
		return "/" + uriEncode(c.bucket) + "/"
	}
	return "/" + uriEncode(c.bucket) + "/" + percentPath(key)
}

// ossBucketEndpoint builds the bucket-scoped endpoint (bucket.host) from a
// region endpoint like https://oss-cn-beijing.aliyuncs.com.
func ossBucketEndpoint(endpoint, bucket string) (*url.URL, error) {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "https://" + endpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("OSS Endpoint 无效，请填写类似 https://oss-cn-beijing.aliyuncs.com: %w", err)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("OSS Endpoint 缺少 host")
	}
	prefix := bucket + "."
	if !strings.HasPrefix(u.Host, prefix) {
		u.Host = prefix + u.Host
	}
	return u, nil
}

// InferOSSRegion extracts the region from an endpoint host like
// oss-cn-beijing.aliyuncs.com -> cn-beijing. Returns "" when not derivable.
func InferOSSRegion(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return ""
	}
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "https://" + endpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return ""
	}
	host := strings.ToLower(u.Host)
	const marker = "oss-"
	idx := strings.Index(host, marker)
	if idx < 0 {
		return ""
	}
	rest := host[idx+len(marker):]
	end := strings.Index(rest, ".")
	if end <= 0 {
		return ""
	}
	return rest[:end]
}

// --- signing helpers ---

func hmacSHA256(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(value))
	return mac.Sum(nil)
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func v4SigningKey(secret, date, region string) []byte {
	dateKey := hmacSHA256([]byte("aliyun_v4"+secret), date)
	dateRegionKey := hmacSHA256(dateKey, region)
	dateRegionServiceKey := hmacSHA256(dateRegionKey, "oss")
	return hmacSHA256(dateRegionServiceKey, "aliyun_v4_request")
}

func canonicalQuery(query map[string]string) string {
	if len(query) == 0 {
		return ""
	}
	keys := make([]string, 0, len(query))
	for k := range query {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		v := query[k]
		if v == "" {
			parts = append(parts, uriEncode(k))
		} else {
			parts = append(parts, uriEncode(k)+"="+uriEncode(v))
		}
	}
	return strings.Join(parts, "&")
}

func percentPath(value string) string {
	parts := strings.Split(value, "/")
	for i, p := range parts {
		parts[i] = uriEncode(p)
	}
	return strings.Join(parts, "/")
}

// uriEncode percent-encodes per RFC 3986, leaving only unreserved characters.
func uriEncode(value string) string {
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') ||
			ch == '-' || ch == '_' || ch == '.' || ch == '~' {
			b.WriteByte(ch)
		} else {
			b.WriteString(fmt.Sprintf("%%%02X", ch))
		}
	}
	return b.String()
}
