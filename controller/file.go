package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"go-file/common"
	"go-file/model"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type FileDeleteRequest struct {
	Id   int
	Link string
	//Token string
}

// canDeleteResource reports whether the current request may delete a resource
// owned by uploader. Admins may delete anything; other users may only delete
// resources they uploaded themselves.
func canDeleteResource(c *gin.Context, uploader string) bool {
	if c.GetInt("role") == common.RoleAdminUser {
		return true
	}
	username := c.GetString("username")
	return username != "" && username == uploader
}

func UploadFile(c *gin.Context) {
	uploadPath := common.UploadPath
	saveToDatabase := true
	path := c.PostForm("path")
	if path != "" { // Upload to explorer's path
		uploadPath = filepath.Join(common.ExplorerRootPath, path)
		if !common.IsSubPath(common.ExplorerRootPath, uploadPath) {
			// In this case the given path is not valid, so we reset it to ExplorerRootPath.
			uploadPath = common.ExplorerRootPath
		}
		saveToDatabase = false

		// Start a go routine to delete explorer' cache
		if common.ExplorerCacheEnabled {
			go func() {
				ctx := context.Background()
				rdb := common.RDB
				key := "cacheExplorer:" + uploadPath
				rdb.Del(ctx, key)
			}()
		}
	}

	description := c.PostForm("description")
	uploader := c.GetString("username")
	if uploader == "" {
		uploader = "匿名用户"
	}
	currentTime := time.Now().Format("2006-01-02 15:04:05")
	form, err := c.MultipartForm()
	if err != nil {
		c.String(http.StatusBadRequest, fmt.Sprintf("get form err: %s", err.Error()))
		return
	}
	files := form.File["file"]
	createTextFile := false
	if files == nil && description != "" {
		createTextFile = true
		file := &multipart.FileHeader{
			Filename: "text.txt",
			Header:   nil,
			Size:     0,
		}
		files = append(files, file)
	}
	t := time.Now()
	subfolder := t.Format("2006-01")
	err = common.MakeDirIfNotExist(filepath.Join(uploadPath, subfolder))
	if err != nil {
		common.SysError("failed to create folder: " + err.Error())
		c.Status(http.StatusInternalServerError)
		return
	}
	for _, file := range files {
		if limit := common.MaxUploadBytes(); limit > 0 && file.Size > limit {
			c.String(http.StatusRequestEntityTooLarge,
				fmt.Sprintf("文件 %s 超过上传大小限制（%d MB）", file.Filename, common.MaxUploadSizeMB))
			return
		}
		// In case someone wants to upload to other folders.
		filename := filepath.Base(file.Filename)
		link := fmt.Sprintf("%s/%s", subfolder, filename)
		savePath := filepath.Join(uploadPath, subfolder, filename)
		if _, err := os.Stat(savePath); err == nil {
			// File already existed.
			timestamp := t.Format("_2006-01-02_15-04-05")
			ext := filepath.Ext(filename)
			if ext == "" {
				link += timestamp
			} else {
				link = subfolder + "/" + filename[:len(filename)-len(ext)] + timestamp + ext
			}
			savePath = filepath.Join(uploadPath, link)
		}
		if createTextFile {
			// Create a new text file and then write the description to it.
			filename = "文本分享"
			f, err := os.Create(savePath)
			if err != nil {
				message := "failed to create file: " + err.Error()
				common.SysError(message)
				c.String(http.StatusInternalServerError, message)
				return
			}
			_, err = f.WriteString(description)
			if err != nil {
				message := "failed to write text to file: " + err.Error()
				common.SysError(message)
				c.String(http.StatusInternalServerError, message)
				return
			}
			descriptionRune := []rune(description)
			if len(descriptionRune) > common.AbstractTextLength {
				description = fmt.Sprintf("内容摘要：%s...", string(descriptionRune[:common.AbstractTextLength]))
			}
		} else {
			if err := c.SaveUploadedFile(file, savePath); err != nil {
				message := "failed to save uploaded file: " + err.Error()
				common.SysError(message)
				c.String(http.StatusInternalServerError, message)
				return
			}
		}
		if saveToDatabase {
			var size int64
			if fi, statErr := os.Stat(savePath); statErr == nil {
				size = fi.Size()
			}
			fileObj := &model.File{
				Description:  description,
				Uploader:     uploader,
				Time:         currentTime,
				Link:         link,
				Filename:     filename,
				Size:         size,
				StorageState: common.StorageLocal,
			}
			err = fileObj.Insert()
			if err != nil {
				common.SysError("failed to insert file to database: " + err.Error())
				continue
			}
		}
	}
	c.Redirect(http.StatusSeeOther, "./")
}

// storeUploadedFile saves one multipart file under the canonical YYYY-MM
// subfolder of root, de-duplicating the name with a timestamp on collision, and
// returns the relative link and absolute path actually used. It mirrors the
// naming convention of UploadFile so AI-uploaded files are indistinguishable
// from web-uploaded ones.
func storeUploadedFile(c *gin.Context, root string, file *multipart.FileHeader) (link, savePath string, err error) {
	t := time.Now()
	subfolder := t.Format("2006-01")
	if err = common.MakeDirIfNotExist(filepath.Join(root, subfolder)); err != nil {
		return "", "", fmt.Errorf("create folder: %w", err)
	}
	filename := filepath.Base(file.Filename)
	link = fmt.Sprintf("%s/%s", subfolder, filename)
	savePath = filepath.Join(root, subfolder, filename)
	if _, statErr := os.Stat(savePath); statErr == nil {
		// Name collision: suffix a timestamp, preserving the extension.
		timestamp := t.Format("_2006-01-02_15-04-05")
		ext := filepath.Ext(filename)
		if ext == "" {
			link += timestamp
		} else {
			link = subfolder + "/" + filename[:len(filename)-len(ext)] + timestamp + ext
		}
		savePath = filepath.Join(root, link)
	}
	if err = c.SaveUploadedFile(file, savePath); err != nil {
		return "", "", fmt.Errorf("save uploaded file: %w", err)
	}
	return link, savePath, nil
}

func DeleteFile(c *gin.Context) {
	var deleteRequest FileDeleteRequest
	err := json.NewDecoder(c.Request.Body).Decode(&deleteRequest)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}

	fileObj := &model.File{
		Id: deleteRequest.Id,
	}
	model.DB.Where("id = ?", deleteRequest.Id).First(&fileObj)
	if fileObj.Id == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "文件不存在",
		})
		return
	}
	if !canDeleteResource(c, fileObj.Uploader) {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "无权删除他人上传的文件",
		})
		return
	}
	if err = fileObj.Delete(); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "文件删除成功",
	})
}

func DownloadFile(c *gin.Context) {
	path := c.Param("filepath")
	subfolder, filename := filepath.Split(path)
	link := filename // Keep compatibility with old version
	if subfolder != "/" {
		link = fmt.Sprintf("%s%s", subfolder, filename)
		link = strings.TrimPrefix(link, "/")
	}
	fullPath := filepath.Join(common.UploadPath, subfolder, filename)
	if !common.IsSubPath(common.UploadPath, fullPath) {
		// We may being attacked!
		c.Status(403)
		return
	}
	// If the file has been archived to cold storage, pull it back from the WebDAV
	// channel before serving. Subsequent downloads within the retention window
	// hit the local copy directly.
	if fileObj, err := model.FileByLink(link); err == nil && fileObj != nil && fileObj.IsArchived() {
		restored, rerr := EnsureLocalCopy(fileObj)
		if rerr != nil {
			common.SysError("restore archived file " + link + " failed: " + rerr.Error())
			c.String(http.StatusBadGateway, "文件已归档至云端，取回失败，请稍后重试")
			return
		}
		fullPath = restored
	}
	if strings.HasSuffix(fullPath, ".txt") && common.IsMobileUserAgent(c.Request.UserAgent()) {
		content, err := os.ReadFile(fullPath)
		if err != nil {
			c.Status(404)
			return
		}
		c.HTML(http.StatusOK, "text-copy.html", gin.H{
			"content": string(content),
		})
	} else {
		c.File(fullPath)
	}
	// Update download counter — but only for a fresh, full download. An HTML5
	// media player streams a file with many Range requests (seek/buffer), so
	// counting every request inflates the number wildly (a 2h video play can
	// register 100+ "downloads"). Skip seek/continuation ranges (bytes=N- with
	// N>0) and only count the start of a transfer.
	if c.Writer.Status() < 400 && isFreshDownload(c.Request) {
		go func() {
			model.UpdateDownloadCounter(link)
		}()
	}
}

// isFreshDownload reports whether the request is the start of a download rather
// than a seek/continuation chunk of an in-progress media stream. A request with
// no Range header is a full download; a single Range starting at byte 0 is the
// first chunk of a playback session (counted once). Anything else — a range
// starting past 0, a suffix range (bytes=-N), or a multi-range request — is
// treated as streaming noise and not counted.
func isFreshDownload(r *http.Request) bool {
	rangeHeader := r.Header.Get("Range")
	if rangeHeader == "" {
		return true
	}
	const prefix = "bytes="
	if !strings.HasPrefix(rangeHeader, prefix) {
		return false
	}
	spec := strings.TrimPrefix(rangeHeader, prefix)
	if strings.Contains(spec, ",") {
		return false // multi-range → partial
	}
	dash := strings.IndexByte(spec, '-')
	if dash < 0 {
		return false
	}
	return strings.TrimSpace(spec[:dash]) == "0"
}
