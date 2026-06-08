package common

import "testing"

func TestUriEncode(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"abcABC123", "abcABC123"},
		{"-_.~", "-_.~"},
		{"a b", "a%20b"},
		{"a/b", "a%2Fb"},
		{"中文", "%E4%B8%AD%E6%96%87"},
		{"a+b=c", "a%2Bb%3Dc"},
	}
	for _, c := range cases {
		if got := uriEncode(c.in); got != c.want {
			t.Errorf("uriEncode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPercentPath(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// Slashes separate path segments and are preserved; each segment is encoded.
		{"2024-01/report.pdf", "2024-01/report.pdf"},
		{"a b/c d", "a%20b/c%20d"},
		{"plain", "plain"},
	}
	for _, c := range cases {
		if got := percentPath(c.in); got != c.want {
			t.Errorf("percentPath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestInferOSSRegion(t *testing.T) {
	cases := []struct {
		endpoint string
		want     string
	}{
		{"https://oss-cn-beijing.aliyuncs.com", "cn-beijing"},
		{"oss-cn-hangzhou.aliyuncs.com", "cn-hangzhou"},
		{"https://oss-us-west-1.aliyuncs.com", "us-west-1"},
		{"https://example.com", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := InferOSSRegion(c.endpoint); got != c.want {
			t.Errorf("InferOSSRegion(%q) = %q, want %q", c.endpoint, got, c.want)
		}
	}
}

func TestOSSBucketEndpoint(t *testing.T) {
	u, err := ossBucketEndpoint("https://oss-cn-beijing.aliyuncs.com", "my-bucket")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := u.Host; got != "my-bucket.oss-cn-beijing.aliyuncs.com" {
		t.Errorf("host = %q, want my-bucket.oss-cn-beijing.aliyuncs.com", got)
	}
	// An endpoint that already carries the bucket prefix must not be doubled.
	u2, err := ossBucketEndpoint("https://my-bucket.oss-cn-beijing.aliyuncs.com", "my-bucket")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := u2.Host; got != "my-bucket.oss-cn-beijing.aliyuncs.com" {
		t.Errorf("host = %q, want no duplicate bucket prefix", got)
	}
}

func TestV4SigningKeyDeterministic(t *testing.T) {
	a := v4SigningKey("secret", "20240101", "cn-beijing")
	b := v4SigningKey("secret", "20240101", "cn-beijing")
	if string(a) != string(b) {
		t.Error("v4SigningKey must be deterministic for identical inputs")
	}
	c := v4SigningKey("secret", "20240102", "cn-beijing")
	if string(a) == string(c) {
		t.Error("v4SigningKey must differ when the date differs")
	}
}

func TestCanonicalQuery(t *testing.T) {
	got := canonicalQuery(map[string]string{"uploads": "", "partNumber": "2"})
	// Keys are sorted; empty values render as the bare key.
	if got != "partNumber=2&uploads" {
		t.Errorf("canonicalQuery = %q, want partNumber=2&uploads", got)
	}
	if canonicalQuery(nil) != "" {
		t.Error("canonicalQuery(nil) must be empty")
	}
}
