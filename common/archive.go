package common

import (
	"os"
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

// LoadArchiveSecretsFromEnv reads the sensitive archive credentials from the
// environment. Call this once at startup. Secrets are intentionally kept out of
// the Option table and the management UI.
func LoadArchiveSecretsFromEnv() {
	archiveSecretMu.Lock()
	defer archiveSecretMu.Unlock()
	ossAccessKeySecret = strings.TrimSpace(os.Getenv("OSS_ACCESS_KEY_SECRET"))
	ossSecurityToken = strings.TrimSpace(os.Getenv("OSS_SECURITY_TOKEN"))
	webDAVPassword = os.Getenv("WEBDAV_PASSWORD")
	// Allow the AccessKey ID to come from the environment too, so a deployment
	// can keep the whole OSS credential pair out of the database if it prefers.
	if id := strings.TrimSpace(os.Getenv("OSS_ACCESS_KEY_ID")); id != "" {
		OSSAccessKeyID = id
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
