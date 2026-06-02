package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go-file/common"
)

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
