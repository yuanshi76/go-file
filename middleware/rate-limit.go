package middleware

import (
	"context"
	"github.com/gin-gonic/gin"
	"go-file/common"
	"net/http"
	"strings"
	"sync"
	"time"
)

// rateLimitHelper is a correct fixed-window limiter built on INCR + EXPIRE.
//
// The previous list-based implementation was buggy: the key was never given a
// TTL and the 429 branch neither pushed nor trimmed, so once a window filled
// up the "oldest" entry was frozen and the client could stay locked out far
// longer than a minute (and the key leaked in Redis forever). INCR is atomic,
// the EXPIRE on the first hit makes the window self-reset, and a Redis hiccup
// fails open instead of locking everyone out.
func rateLimitHelper(c *gin.Context, maxRequestPerMinute int, mark string) {
	ctx := context.Background()
	rdb := common.RDB
	key := "rateLimit:" + mark + c.ClientIP()
	count, err := rdb.Incr(ctx, key).Result()
	if err != nil {
		// A leftover key from the old list-based limiter holds a non-string
		// value, so INCR reports WRONGTYPE. Self-heal by dropping the stale key
		// and re-counting; the next request on this key behaves normally and the
		// error stops recurring without any manual Redis cleanup.
		if strings.Contains(err.Error(), "WRONGTYPE") {
			rdb.Del(ctx, key)
			count, err = rdb.Incr(ctx, key).Result()
		}
		if err != nil {
			common.SysError("rate limit INCR failed: " + err.Error())
			return // fail open: never block real users because Redis blipped
		}
	}
	if count == 1 {
		// First request in this window: start the 60s countdown.
		rdb.Expire(ctx, key, time.Minute)
	} else if ttl, terr := rdb.TTL(ctx, key).Result(); terr == nil && ttl < 0 {
		// Defensive: a key left without a TTL (e.g. a crash between INCR and
		// EXPIRE) would otherwise block this IP forever. Re-arm the window.
		rdb.Expire(ctx, key, time.Minute)
	}
	if count > int64(maxRequestPerMinute) {
		c.Status(http.StatusTooManyRequests)
		c.Abort()
	}
}

// isStaticAsset reports whether the request targets an embedded static file
// (CSS/JS/icons). A single HTML page pulls in several of these, so counting
// them against the per-minute web limit lets a few honest refreshes trip a
// 429 that then locks the user out of the page itself. They are served from
// memory and carry no abuse risk, so they are exempt from the global limit.
func isStaticAsset(path string) bool {
	return strings.HasPrefix(path, "/public/")
}

// memoryRateLimiter is a process-local sliding-window limiter used as a fallback
// when Redis is disabled, so brute-force protection on critical paths (login)
// still applies in the default single-node deployment.
type memoryRateLimiter struct {
	mu    sync.Mutex
	store map[string][]int64 // key -> request timestamps (unix nano)
}

func (m *memoryRateLimiter) allow(key string, maxRequests int, window time.Duration) bool {
	now := time.Now().UnixNano()
	cutoff := now - int64(window)
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := m.store[key][:0]
	for _, t := range m.store[key] {
		if t > cutoff {
			kept = append(kept, t)
		}
	}
	if len(kept) >= maxRequests {
		m.store[key] = kept
		return false
	}
	m.store[key] = append(kept, now)
	return true
}

var criticalMemoryLimiter = &memoryRateLimiter{store: make(map[string][]int64)}

func memoryRateLimit(c *gin.Context, maxRequestPerMinute int, mark string) {
	if !criticalMemoryLimiter.allow(mark+c.ClientIP(), maxRequestPerMinute, time.Minute) {
		c.Status(http.StatusTooManyRequests)
		c.Abort()
	}
}

func GlobalWebRateLimit() func(c *gin.Context) {
	return func(c *gin.Context) {
		if isStaticAsset(c.Request.URL.Path) {
			c.Next()
			return
		}
		if common.RedisEnabled {
			rateLimitHelper(c, common.GlobalWebRateLimit, "GW")
		} else {
			c.Next()
		}
	}
}

func GlobalAPIRateLimit() func(c *gin.Context) {
	return func(c *gin.Context) {
		if common.RedisEnabled {
			rateLimitHelper(c, common.GlobalApiRateLimit, "GA")
		} else {
			c.Next()
		}
	}
}

func CriticalRateLimit() func(c *gin.Context) {
	return func(c *gin.Context) {
		if common.RedisEnabled {
			rateLimitHelper(c, common.CriticalRateLimit, "CT")
		} else {
			memoryRateLimit(c, common.CriticalRateLimit, "CT")
		}
	}
}

func DownloadRateLimit() func(c *gin.Context) {
	return func(c *gin.Context) {
		if common.RedisEnabled {
			rateLimitHelper(c, common.DownloadRateLimit, "CM")
		} else {
			c.Next()
		}
	}
}
