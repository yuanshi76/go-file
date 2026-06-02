package controller

import (
	"encoding/json"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go-file/common"
	"go-file/model"
	"net/http"
	"net/url"
	"strings"
)

func Login(c *gin.Context) {
	username := c.PostForm("username")
	password := c.PostForm("password")
	user := model.User{
		Username: username,
		Password: password,
	}
	if err := user.ValidateAndFill(); err != nil || user.Status != common.UserStatusEnabled {
		c.HTML(http.StatusForbidden, "login.html", gin.H{
			"message":  "用户名或密码错误，或者该用户已被封禁",
			"option":   common.OptionMap,
			"username": c.GetString("username"),
		})
		return
	}

	session := sessions.Default(c)
	session.Set("id", user.Id)
	session.Set("username", username)
	session.Set("role", user.Role)
	err := session.Save()
	if err != nil {
		c.HTML(http.StatusForbidden, "login.html", gin.H{
			"message":  "无法保存会话信息，请重试",
			"option":   common.OptionMap,
			"username": c.GetString("username"),
		})
		return
	}
	c.Redirect(http.StatusFound, safeRedirectTarget(c.Request.Referer()))
	return
}

// safeRedirectTarget restricts post-login redirects to same-site relative
// paths to prevent open redirect attacks via a forged Referer header.
func safeRedirectTarget(referer string) string {
	const fallback = "/"
	if referer == "" {
		return fallback
	}
	u, err := url.Parse(referer)
	if err != nil {
		return fallback
	}
	target := u.Path
	if target == "" || !strings.HasPrefix(target, "/") || strings.HasPrefix(target, "//") {
		return fallback
	}
	if strings.HasSuffix(target, "/login") {
		return fallback
	}
	if u.RawQuery != "" {
		target += "?" + u.RawQuery
	}
	return target
}

func Logout(c *gin.Context) {
	session := sessions.Default(c)
	session.Options(sessions.Options{MaxAge: -1})
	session.Save()
	c.Redirect(http.StatusFound, "/login")
}

type UpdateSelfRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Password    string `json:"password"`
}

func UpdateSelf(c *gin.Context) {
	var req UpdateSelfRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	id := c.GetInt("id")
	updates := make(map[string]interface{})

	req.Username = strings.TrimSpace(req.Username)
	if req.Username != "" {
		if len(req.Username) < 3 || len(req.Username) > 32 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "用户名长度需在 3 到 32 个字符之间"})
			return
		}
		if model.IsUsernameTaken(req.Username, id) {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "该用户名已被占用"})
			return
		}
		updates["username"] = req.Username
	}

	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if req.DisplayName != "" {
		// Strip angle brackets to avoid stored XSS when the name is rendered.
		updates["display_name"] = common.SanitizeText(req.DisplayName)
	}

	if req.Password != "" {
		if len(req.Password) < 6 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "密码长度至少为 6 个字符"})
			return
		}
		hashed, err := model.HashPassword(req.Password)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		updates["password"] = hashed
	}

	if len(updates) == 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "没有需要更新的内容"})
		return
	}

	if err := model.UpdateUserFields(id, updates); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
	return
}

// CreateUser Only admin user can call this, so we can trust it
func CreateUser(c *gin.Context) {
	var user model.User
	err := json.NewDecoder(c.Request.Body).Decode(&user)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	user.Username = strings.TrimSpace(user.Username)
	if len(user.Username) < 3 || len(user.Username) > 32 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "用户名长度需在 3 到 32 个字符之间"})
		return
	}
	if len(user.Password) < 6 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "密码长度至少为 6 个字符"})
		return
	}
	user.DisplayName = common.SanitizeText(user.Username)

	if err := user.Insert(); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

type ManageRequest struct {
	Username string `json:"username"`
	Action   string `json:"action"`
}

// ManageUser Only admin user can do this
func ManageUser(c *gin.Context) {
	var req ManageRequest
	err := json.NewDecoder(c.Request.Body).Decode(&req)

	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	user := model.User{
		Username: req.Username,
	}
	// Fill attributes
	model.DB.Where(&user).First(&user)
	if user.Id == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "用户不存在",
		})
		return
	}
	switch req.Action {
	case "disable":
		user.Status = common.UserStatusDisabled
	case "enable":
		user.Status = common.UserStatusEnabled
	case "delete":
		if err := user.Delete(); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "promote":
		user.Role = common.RoleAdminUser
	case "demote":
		user.Role = common.RoleCommonUser
	}

	if err := user.Update(); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func GenerateNewUserToken(c *gin.Context) {
	var user model.User
	user.Id = c.GetInt("id")
	// Fill attributes
	model.DB.Where(&user).First(&user)
	if user.Id == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "用户不存在",
		})
		return
	}
	user.Token = uuid.New().String()
	user.Token = strings.Replace(user.Token, "-", "", -1)

	if err := user.Update(); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    user.Token,
	})
	return
}
