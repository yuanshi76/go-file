package main

import (
	"fmt"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-contrib/sessions/redis"
	"github.com/gin-gonic/gin"
	"github.com/jinzhu/gorm"
	"go-file/common"
	"go-file/controller"
	"go-file/middleware"
	"go-file/model"
	"go-file/router"
	"html/template"
	"net/http"
	"os"
	"strconv"
)

func loadTemplate() *template.Template {
	var funcMap = template.FuncMap{
		"unescape": common.UnescapeHTML,
	}
	t := template.Must(template.New("").Funcs(funcMap).ParseFS(common.FS, "public/*.html"))
	return t
}

func main() {
	common.SetupGinLog()
	common.SysLog(fmt.Sprintf("Go File %s started at port %d", common.Version, *common.Port))
	if os.Getenv("GIN_MODE") != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}
	// Initialize SQL Database
	db, err := model.InitDB()
	if err != nil {
		common.FatalLog(err)
	}
	defer func(db *gorm.DB) {
		err := db.Close()
		if err != nil {
			common.FatalLog("failed to close database: " + err.Error())
		}
	}(db)

	// Initialize Redis
	err = common.InitRedisClient()
	if err != nil {
		common.FatalLog(err)
	}

	// Initialize options
	model.InitOptionMap()

	// Load archive credentials and OSS/WebDAV configuration from the environment
	// after options, so env-provided values take precedence over any persisted
	// (settings-page) overrides.
	common.LoadArchiveFromEnv()
	// Start the cold-storage archival worker (no-op until enabled & configured).
	controller.StartArchiveWorker()

	// Persist the session secret so cookie sessions survive restarts.
	common.InitSessionSecret()

	// Initialize HTTP server
	server := gin.Default()
	// gin.Default() defaults MaxMultipartMemory to 32MB, which means any upload
	// up to 32MB is buffered entirely in RAM before touching disk. On small-memory
	// hosts that transient spike can exceed available memory and get the process
	// OOM-killed by the host (cgroup OOMKilled stays false in that case), which
	// looks like a crash-restart loop on uploads larger than ~30MB. Cap the in-RAM
	// buffer at 8MB so larger uploads stream-spill to a temp file on disk instead.
	server.MaxMultipartMemory = 8 << 20 // 8MB
	server.SetHTMLTemplate(loadTemplate())

	// Initialize session store
	var store sessions.Store
	if common.RedisEnabled {
		opt := common.ParseRedisOption()
		store, _ = redis.NewStore(opt.MinIdleConns, opt.Network, opt.Addr, opt.Password, []byte(common.SessionSecret))
	} else {
		store = cookie.NewStore([]byte(common.SessionSecret))
	}
	store.Options(sessions.Options{
		Path:     "/",
		HttpOnly: true,
		// MaxAge MUST be > 0. The Redis-backed store treats MaxAge <= 0 as
		// "delete this session" on every Save, so a missing MaxAge silently
		// wipes the login right after it is set (cookie store tolerates 0 by
		// keeping the data inline, which masked the bug in local testing).
		// 30 days, matching the store's own default session lifetime.
		MaxAge: 86400 * 30,
		// Lax lets normal top-level navigations carry the cookie while blocking
		// it on cross-site POSTs, which mitigates CSRF. Secure is opt-in via
		// COOKIE_SECURE=true for HTTPS deployments (can't be unconditional or
		// cookies break on plain-HTTP/LAN usage, the common case here).
		SameSite: http.SameSiteLaxMode,
		Secure:   os.Getenv("COOKIE_SECURE") == "true",
	})
	server.Use(sessions.Sessions("session", store))
	server.Use(middleware.CSRFProtect())

	router.SetRouter(server)
	var realPort = os.Getenv("PORT")
	if realPort == "" {
		realPort = strconv.Itoa(*common.Port)
	}
	if *common.Host == "" {
		ip := common.GetIp()
		if ip != "" {
			*common.Host = ip
		} else {
			*common.Host = "localhost"
		}
	}
	serverUrl := "http://" + *common.Host + ":" + realPort + "/"
	if !*common.NoBrowser {
		common.OpenBrowser(serverUrl)
	}
	if *common.EnableP2P {
		go common.StartP2PServer()
	}
	err = server.Run(":" + realPort)
	if err != nil {
		common.FatalLog(err)
	}
}
