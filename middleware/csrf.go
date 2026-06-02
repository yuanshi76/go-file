package middleware

import (
	"github.com/gin-gonic/gin"
	"go-file/common"
	"net/http"
)

// CSRFProtect rejects cross-site state-changing requests. Safe methods and
// token-authenticated requests (which carry credentials in a header an attacker
// cannot set cross-site) are exempt; everything else must come from a trusted
// origin. See common.IsTrustedOrigin for the underlying rule.
func CSRFProtect() func(c *gin.Context) {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
			c.Next()
			return
		}
		if c.Request.Header.Get("Authorization") != "" {
			c.Next()
			return
		}
		if !common.IsTrustedOrigin(c.Request.Header.Get("Origin"), c.Request.Header.Get("Referer"), c.Request.Host) {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Next()
	}
}
