package common

import (
	"embed"
	"flag"
	"fmt"
	"github.com/google/uuid"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

var StartTime = time.Now()
var Version = "v0.0.0"
var OptionMap map[string]string

var ItemsPerPage = 10
var AbstractTextLength = 40

// MaxUploadSizeMB is the per-file upload size limit in megabytes. 0 means no
// limit. Admins can change it from the management settings page.
var MaxUploadSizeMB = 200

// MaxUploadBytes returns the per-file upload size limit in bytes, or 0 when
// uploads are unlimited.
func MaxUploadBytes() int64 {
	if MaxUploadSizeMB <= 0 {
		return 0
	}
	return int64(MaxUploadSizeMB) * 1024 * 1024
}

var ExplorerCacheEnabled = false // After my test, enable this will make the server slower...
var ExplorerCacheTimeout = 600   // Second

var StatEnabled = true
var StatCacheTimeout = 24 // Hour
var StatReqTimeout = 30   // Day
var StatIPNum = 20
var StatURLNum = 20

const (
	RoleGuestUser  = 0
	RoleCommonUser = 1
	RoleAdminUser  = 10
)

var (
	FileUploadPermission    = RoleGuestUser
	FileDownloadPermission  = RoleGuestUser
	ImageUploadPermission   = RoleGuestUser
	ImageDownloadPermission = RoleGuestUser
	// VideoDownloadPermission defaults to RoleCommonUser so stored videos are
	// not exposed to anonymous visitors unless an admin lowers it.
	VideoDownloadPermission = RoleCommonUser
)

// Per-IP request budgets, counted per minute. These must leave room for normal
// browsing: one page can fire several sub-requests, and an image gallery or a
// burst of file clicks all share DownloadRateLimit, so the old value of 10 was
// tripped by ordinary use. Static assets under /public/ are exempt entirely
// (see middleware/rate-limit.go), so GlobalWebRateLimit now only counts page
// navigations. CriticalRateLimit stays strict on purpose: it guards the login
// endpoint against brute force.
var (
	GlobalApiRateLimit = 60
	GlobalWebRateLimit = 120
	DownloadRateLimit  = 120
	CriticalRateLimit  = 5
)

const (
	UserStatusEnabled  = 1
	UserStatusDisabled = 2 // don't use 0
)

var (
	Port         = flag.Int("port", 3000, "Specify the server listening port.")
	Host         = flag.String("host", "", "The server's IP address or domain.")
	Path         = flag.String("path", "", "Specify a local path to public.")
	VideoPath    = flag.String("video", "", "Specify a folder containing videos to be made public.")
	NoBrowser    = flag.Bool("no-browser", false, "Do not open browser automatically.")
	PrintVersion = flag.Bool("version", false, "Print version information.")
	EnableP2P    = flag.Bool("enable-p2p", false, "Enable peer-to-peer relay or not.")
	P2PPort      = flag.Int("p2p-port", 9377, "Specify the P2P listening port.")
	LogDir       = flag.String("log-dir", "", "Specify the directory for log files.")
	PrintHelp    = flag.Bool("help", false, "Print usage information.")
)

// UploadPath Maybe override by ENV_VAR
var UploadPath = "upload"
var ExplorerRootPath = UploadPath
var ImageUploadPath = "upload/images"
var VideoServePath = "upload"

//go:embed public
var FS embed.FS

var SessionSecret = uuid.New().String()
var sessionSecretFromEnv bool

// SessionSecretPath is where the auto-generated session secret is persisted so
// that cookie sessions survive restarts. Ignored when SESSION_SECRET is set.
var SessionSecretPath = "session.key"

var SQLitePath = "go-file.db"

func printHelp() {
	fmt.Println(fmt.Sprintf("Go File %s - A simple file sharing tool.", Version))
	fmt.Println("Copyright (C) 2023 JustSong. All rights reserved.")
	fmt.Println("GitHub: https://github.com/yuanshi76/go-file")
	fmt.Println("Usage: go-file [options]")
	fmt.Println("Options:")
	flag.CommandLine.VisitAll(func(f *flag.Flag) {
		name := fmt.Sprintf("-%s", f.Name)
		usage := strings.Replace(f.Usage, "\n", "\n    ", -1)
		fmt.Printf("        -%-14s%s\n", name, usage)
	})
	os.Exit(0)
}

// isTestBinary reports whether the process is a `go test` binary. Such binaries
// are named "*.test" (or "*.test.exe" on Windows) and inject -test.* flags that
// this package's flag set does not define, so flag.Parse() must be skipped there.
func isTestBinary() bool {
	arg0 := os.Args[0]
	return strings.HasSuffix(arg0, ".test") || strings.HasSuffix(arg0, ".test.exe")
}

func init() {
	if !isTestBinary() {
		flag.Parse()
	}

	if *PrintHelp {
		printHelp()
	}

	if *PrintVersion {
		fmt.Println(Version)
		os.Exit(0)
	}

	if os.Getenv("SESSION_SECRET") != "" {
		SessionSecret = os.Getenv("SESSION_SECRET")
		sessionSecretFromEnv = true
	}
	if os.Getenv("SESSION_SECRET_PATH") != "" {
		SessionSecretPath = os.Getenv("SESSION_SECRET_PATH")
	}
	if os.Getenv("SQLITE_PATH") != "" {
		SQLitePath = os.Getenv("SQLITE_PATH")
	}
	if os.Getenv("UPLOAD_PATH") != "" {
		UploadPath = os.Getenv("UPLOAD_PATH")
		ExplorerRootPath = UploadPath
		ImageUploadPath = path.Join(UploadPath, "images")
		VideoServePath = UploadPath
	}
	if *Path != "" {
		ExplorerRootPath = *Path
	}
	if *VideoPath != "" {
		VideoServePath = *VideoPath
	}

	ExplorerRootPath, _ = filepath.Abs(ExplorerRootPath)
	VideoServePath, _ = filepath.Abs(VideoServePath)
	ImageUploadPath, _ = filepath.Abs(ImageUploadPath)

	if _, err := os.Stat(UploadPath); os.IsNotExist(err) {
		_ = os.Mkdir(UploadPath, 0750)
	}
	if _, err := os.Stat(ImageUploadPath); os.IsNotExist(err) {
		_ = os.Mkdir(ImageUploadPath, 0750)
	}
}
