package controller

import (
	"fmt"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"go-file/common"
	"go-file/model"
	"net/http"
	"runtime"
	"strconv"
	"time"
)

// viewerRole returns the role of the current session user, defaulting to guest
// when no one is logged in.
func viewerRole(c *gin.Context) int {
	session := sessions.Default(c)
	if role, ok := session.Get("role").(int); ok {
		return role
	}
	return common.RoleGuestUser
}

func GetIndexPage(c *gin.Context) {
	// The home page lists file names/metadata, so it must honor the same
	// permission as downloading. Otherwise raising FileDownloadPermission to
	// require login still leaks the listing to anonymous visitors here, even
	// though /explorer and the download routes are gated.
	if viewerRole(c) < common.FileDownloadPermission {
		c.HTML(http.StatusForbidden, "error.html", gin.H{
			"message":  "请登录后查看文件列表",
			"option":   common.OptionMap,
			"username": c.GetString("username"),
		})
		return
	}

	query := c.Query("query")
	isQuery := query != ""
	p, _ := strconv.Atoi(c.Query("p"))
	if p < 0 {
		p = 0
	}
	next := p + 1
	prev := common.IntMax(0, p-1)

	startIdx := p * common.ItemsPerPage

	files, err := model.QueryFiles(query, startIdx)
	if err != nil {
		c.HTML(http.StatusOK, "error.html", gin.H{
			"message":  err.Error(),
			"option":   common.OptionMap,
			"username": c.GetString("username"),
		})
		return
	}
	if len(files) < common.ItemsPerPage {
		next = 0
	}

	c.HTML(http.StatusOK, "index.html", gin.H{
		"message":  "",
		"option":   common.OptionMap,
		"username": c.GetString("username"),
		"isAdmin":  viewerRole(c) == common.RoleAdminUser,
		"files":    files,
		"isQuery":  isQuery,
		"next":     next,
		"prev":     prev,
	})
}

func GetManagePage(c *gin.Context) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	var uptime = time.Since(common.StartTime)
	session := sessions.Default(c)
	role := session.Get("role")
	c.HTML(http.StatusOK, "manage.html", gin.H{
		"message":                 "",
		"option":                  common.OptionMap,
		"username":                c.GetString("username"),
		"memory":                  fmt.Sprintf("%d MB", m.Sys/1024/1024),
		"uptime":                  common.Seconds2Time(int(uptime.Seconds())),
		"userNum":                 model.CountTable("users"),
		"fileNum":                 model.CountTable("files"),
		"imageNum":                model.CountTable("images"),
		"FileUploadPermission":    common.FileUploadPermission,
		"FileDownloadPermission":  common.FileDownloadPermission,
		"ImageUploadPermission":   common.ImageUploadPermission,
		"ImageDownloadPermission": common.ImageDownloadPermission,
		"VideoDownloadPermission": common.VideoDownloadPermission,
		"MaxUploadSizeMB":         common.MaxUploadSizeMB,
		"isAdmin":                 role == common.RoleAdminUser,
		"StatEnabled":             common.StatEnabled,
		"ArchiveEnabled":          common.ArchiveEnabled,
		"ArchiveAfterDays":        common.ArchiveAfterDays,
		// Whether the env-only secrets are present, so the UI can warn the admin
		// before they try to enable archiving.
		"ArchiveSecretsReady": common.OSSAccessKeySecret() != "" && common.WebDAVPassword() != "",
	})
}

func GetImagePage(c *gin.Context) {
	// The image hosting page is an upload interface, so gate it behind the same
	// permission the /api/image upload endpoint enforces. Without this, raising
	// ImageUploadPermission to require login still serves the upload UI to
	// anonymous visitors, even though the API rejects their uploads.
	if viewerRole(c) < common.ImageUploadPermission {
		c.HTML(http.StatusForbidden, "error.html", gin.H{
			"message":  "请登录后使用图床",
			"option":   common.OptionMap,
			"username": c.GetString("username"),
		})
		return
	}
	c.HTML(http.StatusOK, "image.html", gin.H{
		"message":  "",
		"option":   common.OptionMap,
		"username": c.GetString("username"),
	})
}

func GetLoginPage(c *gin.Context) {
	c.HTML(http.StatusOK, "login.html", gin.H{
		"message":  "",
		"option":   common.OptionMap,
		"username": c.GetString("username"),
	})
}

func GetHelpPage(c *gin.Context) {
	c.HTML(http.StatusOK, "help.html", gin.H{
		"message":  "",
		"option":   common.OptionMap,
		"username": c.GetString("username"),
	})
}

func Get404Page(c *gin.Context) {
	c.HTML(http.StatusOK, "404.html", gin.H{
		"message":  "",
		"option":   common.OptionMap,
		"username": c.GetString("username"),
	})
}
