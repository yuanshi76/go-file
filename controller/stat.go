package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go-file/model"
)

// GetFileStats returns the file-centric statistics that power the manage-page
// dashboard. All metrics are derived from a single scan over the files table,
// so this endpoint works regardless of whether Redis is configured.
func GetFileStats(c *gin.Context) {
	stats, err := model.ComputeFileStats()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
			"data":    nil,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    stats,
	})
}
