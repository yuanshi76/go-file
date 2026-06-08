package common

import "testing"

func TestJoinKey(t *testing.T) {
	cases := []struct {
		prefix string
		name   string
		want   string
	}{
		{"", "a/b.txt", "a/b.txt"},
		{"go-file", "2024-01/a.txt", "go-file/2024-01/a.txt"},
		{"/go-file/", "/2024-01/a.txt", "go-file/2024-01/a.txt"},
		{"go-file", "", "go-file"},
		{"", "", ""},
	}
	for _, c := range cases {
		if got := JoinKey(c.prefix, c.name); got != c.want {
			t.Errorf("JoinKey(%q, %q) = %q, want %q", c.prefix, c.name, got, c.want)
		}
	}
}

func TestArchiveAfterDaysOrDefault(t *testing.T) {
	saved := ArchiveAfterDays
	defer func() { ArchiveAfterDays = saved }()

	cases := []struct {
		set  int
		want int
	}{
		{7, 7},
		{0, 7},
		{-3, 7},
		{30, 30},
	}
	for _, c := range cases {
		ArchiveAfterDays = c.set
		if got := ArchiveAfterDaysOrDefault(); got != c.want {
			t.Errorf("ArchiveAfterDaysOrDefault() with %d = %d, want %d", c.set, got, c.want)
		}
	}
}

func TestArchiveReadyRequiresAllChannels(t *testing.T) {
	// Save and restore all touched globals.
	savedEnabled := ArchiveEnabled
	savedBucket, savedRegion, savedEndpoint, savedID := OSSBucket, OSSRegion, OSSEndpoint, OSSAccessKeyID
	savedDav, savedUser := WebDAVBaseURL, WebDAVUsername
	defer func() {
		ArchiveEnabled = savedEnabled
		OSSBucket, OSSRegion, OSSEndpoint, OSSAccessKeyID = savedBucket, savedRegion, savedEndpoint, savedID
		WebDAVBaseURL, WebDAVUsername = savedDav, savedUser
		archiveSecretMu.Lock()
		ossAccessKeySecret, webDAVPassword = "", ""
		archiveSecretMu.Unlock()
	}()

	// Fully configured.
	ArchiveEnabled = true
	OSSBucket, OSSRegion, OSSEndpoint, OSSAccessKeyID = "b", "cn-beijing", "https://oss-cn-beijing.aliyuncs.com", "id"
	WebDAVBaseURL, WebDAVUsername = "https://dav.example.com", "user"
	archiveSecretMu.Lock()
	ossAccessKeySecret, webDAVPassword = "secret", "pw"
	archiveSecretMu.Unlock()
	if !ArchiveReady() {
		t.Fatal("expected ArchiveReady() true when fully configured")
	}

	// Missing WebDAV password -> not ready.
	archiveSecretMu.Lock()
	webDAVPassword = ""
	archiveSecretMu.Unlock()
	if ArchiveReady() {
		t.Error("expected ArchiveReady() false without WebDAV password")
	}

	// Disabled -> not ready even if configured.
	archiveSecretMu.Lock()
	webDAVPassword = "pw"
	archiveSecretMu.Unlock()
	ArchiveEnabled = false
	if ArchiveReady() {
		t.Error("expected ArchiveReady() false when disabled")
	}
}
