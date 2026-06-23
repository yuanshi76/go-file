package router

import (
	"github.com/gin-gonic/gin"
	"go-file/controller"
	"go-file/middleware"
)

func setApiRouter(router *gin.Engine) {
	router.Use(middleware.GlobalAPIRateLimit())
	router.GET("/status", controller.GetStatus)
	router.POST("/api/file", middleware.FileUploadPermissionCheck(), middleware.UploadSizeLimit(), controller.UploadFile)
	router.POST("/api/image", middleware.ImageUploadPermissionCheck(), middleware.UploadSizeLimit(), controller.UploadImage)
	router.GET("/api/notice", controller.GetNotice)
	// AI API: machine-friendly file discovery + download for AI agents.
	// The manifest is public (self-describing, no secrets); everything else is
	// token-authenticated via the standard Authorization header.
	router.GET("/api/ai/manifest", controller.AIManifest)
	aiAuth := router.Group("/api/ai")
	aiAuth.Use(middleware.ApiAuth())
	{
		// Weak-model-friendly single-step helpers (resolve by name or id).
		aiAuth.GET("/find", controller.AIFindFiles)
		aiAuth.GET("/download", controller.AIDownloadByQuery)
		// Full REST surface.
		aiAuth.GET("/files", controller.AIListFiles)
		aiAuth.POST("/files", middleware.UploadSizeLimit(), controller.AIUploadFile)
		aiAuth.GET("/files/:id", controller.AIGetFile)
		aiAuth.GET("/files/:id/content", controller.AIDownloadFile)
		aiAuth.GET("/stats", controller.AIStats)
	}
	basicAuth := router.Group("/api")
	basicAuth.Use(middleware.ApiAuth())
	{
		basicAuth.DELETE("/file", controller.DeleteFile)
		basicAuth.DELETE("/image", controller.DeleteImage)
		basicAuth.PUT("/user", middleware.NoTokenAuth(), controller.UpdateSelf)
		basicAuth.POST("/token", controller.GenerateNewUserToken)
	}
	adminAuth := router.Group("/api")
	adminAuth.Use(middleware.ApiAdminAuth())
	{
		adminAuth.POST("/user", controller.CreateUser)
		adminAuth.PUT("/manage_user", controller.ManageUser)
		adminAuth.GET("/option", controller.GetOptions)
		adminAuth.PUT("/option", controller.UpdateOption)
		statRouter := adminAuth.Group("/stat")
		{
			statRouter.GET("/files", controller.GetFileStats)
		}
	}
}
