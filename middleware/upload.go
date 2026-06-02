package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go-file/common"
)

// uploadSizeOverhead is extra room above the per-file limit to accommodate
// multipart framing and accompanying form fields (description, path) so a
// single max-size file is not rejected by the request-body cap.
const uploadSizeOverhead = 8 << 20 // 8 MB

// UploadSizeLimit caps the request body so oversized uploads are rejected while
// streaming, before they are fully buffered to disk. It is a no-op when uploads
// are configured as unlimited (MaxUploadSizeMB == 0). The per-file size is still
// validated precisely inside the upload handlers; this is the hard DoS ceiling.
func UploadSizeLimit() func(c *gin.Context) {
	return func(c *gin.Context) {
		if limit := common.MaxUploadBytes(); limit > 0 {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit+uploadSizeOverhead)
		}
		c.Next()
	}
}
