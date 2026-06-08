package common

import (
	"os"
	"strconv"
	"strings"
	"sync"
)

// Storage states persisted on the File row. An empty string is treated the same
// as StorageLocal so that rows created before this feature keep working.
const (
	StorageLocal    = "local"
	StorageArchived = "archived"
)

// Archive (cold-storage) configuration.
//
// Non-secret fields below are configurable from the management settings page and
// persisted through the Option table. The two real secrets (OSS AccessKey Secret
// and the WebDAV password) are loaded from environment variables only and never
// stored in the database, per the project's secret-management policy.
var (
	// ArchiveEnabled turns the periodic archival worker on or off.
	ArchiveEnabled = false
	// ArchiveAfterDays is how many days a file may stay local without being
	// accessed before it is moved to OSS. Counted from the last access time, or
	// from the upload time when never accessed.
	ArchiveAfterDays = 7

	// OSS upload channel (paid) settings.
	OSSBucket      = ""
	OSSRegion      = ""
	OSSEndpoint    = "" // e.g. https://oss-cn-beijing.aliyuncs.com
	OSSAccessKeyID = ""
	OSSKeyPrefix   = "" // remote root prefix inside the bucket

	// WebDAV download channel (free) settings. The WebDAV endpoint must point at
	// the same backing storage as the OSS bucket so that objects uploaded via OSS
	// are readable here.
	WebDAVBaseURL    = ""
	WebDAVUsername   = ""
	WebDAVRootPrefix = ""
)

// archiveSecretMu guards the env-loaded secrets so the worker goroutine and HTTP
// handlers can read them safely.
var archiveSecretMu sync.RWMutex

var (
	ossAccessKeySecret string
	ossSecurityToken   string
	webDAVPassword     string
)

// LoadArchiveFromEnv reads archive credentials AND the non-secret OSS/WebDAV
// configuration from the environment. Call this once at startup, AFTER the
// Option table has been loaded, so that any value provided via the environment
// takes precedence over a value persisted in the database.
//
// Secrets (OSS AccessKey Secret, the optional STS token, and the WebDAV
// password) are only ever read from the environment and never persisted. The
// non-secret fields may also be set from the management settings page; the
// environment simply wins when both are present.
//
// Recognised variables:
//
//	OSS_ACCESS_KEY_SECRET  OSS AccessKey Secret (secret)
//	OSS_SECURITY_TOKEN     STS security token (secret, optional)
//	WEBDAV_PASSWORD        WebDAV password (secret)
//	OSS_ACCESS_KEY_ID      OSS AccessKey ID
//	OSS_BUCKET             OSS bucket name
//	OSS_REGION             OSS region (auto-derived from endpoint when omitted)
//	OSS_ENDPOINT           OSS endpoint, e.g. https://oss-cn-beijing.aliyuncs.com
//	OSS_KEY_PREFIX         remote root prefix inside the bucket
//	WEBDAV_BASE_URL        WebDAV base URL fronting the same storage
//	WEBDAV_USERNAME        WebDAV username
//	WEBDAV_ROOT_PREFIX     WebDAV path prefix
//	ARCHIVE_ENABLED        "true"/"false" to toggle the worker
//	ARCHIVE_AFTER_DAYS     positive integer retention window
func LoadArchiveFromEnv() {
	archiveSecretMu.Lock()
	ossAccessKeySecret = strings.TrimSpace(os.Getenv("OSS_ACCESS_KEY_SECRET"))
	ossSecurityToken = strings.TrimSpace(os.Getenv("OSS_SECURITY_TOKEN"))
	webDAVPassword = os.Getenv("WEBDAV_PASSWORD")
	archiveSecretMu.Unlock()

	// Non-secret OSS upload channel settings.
	setStringFromEnv("OSS_ACCESS_KEY_ID", &OSSAccessKeyID)
	setStringFromEnv("OSS_BUCKET", &OSSBucket)
	setStringFromEnv("OSS_REGION", &OSSRegion)
	setStringFromEnv("OSS_ENDPOINT", &OSSEndpoint)
	setStringFromEnv("OSS_KEY_PREFIX", &OSSKeyPrefix)

	// Non-secret WebDAV download channel settings.
	setStringFromEnv("WEBDAV_BASE_URL", &WebDAVBaseURL)
	setStringFromEnv("WEBDAV_USERNAME", &WebDAVUsername)
	setStringFromEnv("WEBDAV_ROOT_PREFIX", &WebDAVRootPrefix)

	// Derive the region from the endpoint when only the endpoint was supplied.
	if strings.TrimSpace(OSSRegion) == "" && strings.TrimSpace(OSSEndpoint) != "" {
		OSSRegion = InferOSSRegion(OSSEndpoint)
	}

	// Optional behavioural toggles.
	if v := strings.TrimSpace(os.Getenv("ARCHIVE_ENABLED")); v != "" {
		ArchiveEnabled = strings.EqualFold(v, "true")
	}
	if v := strings.TrimSpace(os.Getenv("ARCHIVE_AFTER_DAYS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			ArchiveAfterDays = n
		}
	}
}

// setStringFromEnv overwrites *dst with the trimmed value of the named env var
// when that variable is set to a non-empty value; otherwise *dst is left
// unchanged so any previously loaded (DB/default) value survives.
func setStringFromEnv(key string, dst *string) {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		*dst = v
	}
}

// OSSAccessKeySecret returns the env-loaded OSS AccessKey Secret.
func OSSAccessKeySecret() string {
	archiveSecretMu.RLock()
	defer archiveSecretMu.RUnlock()
	return ossAccessKeySecret
}

// OSSSecurityToken returns the optional STS security token, or "" when unset.
func OSSSecurityToken() string {
	archiveSecretMu.RLock()
	defer archiveSecretMu.RUnlock()
	return ossSecurityToken
}

// WebDAVPassword returns the env-loaded WebDAV password.
func WebDAVPassword() string {
	archiveSecretMu.RLock()
	defer archiveSecretMu.RUnlock()
	return webDAVPassword
}

// OSSConfigReady reports whether enough OSS configuration is present to upload.
func OSSConfigReady() bool {
	return strings.TrimSpace(OSSBucket) != "" &&
		strings.TrimSpace(OSSEndpoint) != "" &&
		strings.TrimSpace(OSSAccessKeyID) != "" &&
		strings.TrimSpace(OSSRegion) != "" &&
		OSSAccessKeySecret() != ""
}

// WebDAVConfigReady reports whether enough WebDAV configuration is present to
// download.
func WebDAVConfigReady() bool {
	return strings.TrimSpace(WebDAVBaseURL) != "" &&
		strings.TrimSpace(WebDAVUsername) != "" &&
		WebDAVPassword() != ""
}

// ArchiveReady reports whether the archival pipeline is fully configured: both
// the OSS upload channel and the WebDAV download channel must be usable,
// otherwise an archived file could become unreachable.
func ArchiveReady() bool {
	return ArchiveEnabled && OSSConfigReady() && WebDAVConfigReady()
}

// JoinKey joins a prefix and a relative path into an object key using forward
// slashes, trimming stray separators. An empty prefix yields the bare name.
func JoinKey(prefix, name string) string {
	prefix = strings.Trim(prefix, "/")
	name = strings.TrimLeft(name, "/")
	if prefix == "" {
		return name
	}
	if name == "" {
		return prefix
	}
	return prefix + "/" + name
}

// ArchiveAfterDaysOrDefault returns a sane positive day count.
func ArchiveAfterDaysOrDefault() int {
	if ArchiveAfterDays <= 0 {
		return 7
	}
	return ArchiveAfterDays
}
