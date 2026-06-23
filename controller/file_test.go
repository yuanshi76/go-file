package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go-file/common"
)

func TestIsFreshDownload(t *testing.T) {
	tests := []struct {
		name      string
		rangeHdr  string
		setHeader bool
		want      bool
	}{
		{"no range header is full download", "", false, true},
		{"range from zero counts once", "bytes=0-", true, true},
		{"range from zero with end", "bytes=0-1023", true, true},
		{"seek range does not count", "bytes=1048576-", true, false},
		{"mid range does not count", "bytes=500-999", true, false},
		{"suffix range does not count", "bytes=-500", true, false},
		{"multi range does not count", "bytes=0-99,200-299", true, false},
		{"non-bytes unit does not count", "items=0-5", true, false},
		{"garbage does not count", "0-", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "http://example.com/d/clip.mp4", nil)
			if tt.setHeader {
				r.Header.Set("Range", tt.rangeHdr)
			}
			if got := isFreshDownload(r); got != tt.want {
				t.Errorf("isFreshDownload(Range=%q) = %v, want %v", tt.rangeHdr, got, tt.want)
			}
		})
	}
}

func TestCanDeleteResource(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name     string
		role     int
		username string
		uploader string
		want     bool
	}{
		{"admin deletes others", common.RoleAdminUser, "admin", "alice", true},
		{"admin deletes own", common.RoleAdminUser, "admin", "admin", true},
		{"owner deletes own", common.RoleCommonUser, "alice", "alice", true},
		{"common cannot delete others", common.RoleCommonUser, "alice", "bob", false},
		{"empty username cannot match empty uploader", common.RoleCommonUser, "", "", false},
		{"guest cannot delete", common.RoleGuestUser, "", "alice", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set("role", tt.role)
			c.Set("username", tt.username)
			if got := canDeleteResource(c, tt.uploader); got != tt.want {
				t.Errorf("canDeleteResource(role=%d, user=%q, uploader=%q) = %v, want %v",
					tt.role, tt.username, tt.uploader, got, tt.want)
			}
		})
	}
}
