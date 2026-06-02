package middleware

import (
	"context"
	"github.com/gin-gonic/gin"
	"go-file/common"
	"net/http"
	"sync"
	"time"
)

var timeFormat = "2006-01-02T15:04:05.000Z"

func rateLimitHelper(c *gin.Context, maxRequestPerMinute int, mark string) {
	ctx := context.Background()
	rdb := common.RDB
	key := "rateLimit:" + mark + c.ClientIP()
	listLength, err := rdb.LLen(ctx, key).Result()
	if err != nil {
		common.SysError("rate limit LLen failed: " + err.Error())
	}
	if listLength < int64(maxRequestPerMinute) {
		rdb.LPush(ctx, key, time.Now().Format(timeFormat))
	} else {
		oldTimeStr, _ := rdb.LIndex(ctx, key, -1).Result()
		oldTime, err := time.Parse(timeFormat, oldTimeStr)
		if err != nil {
			common.SysError("rate limit parse time failed: " + err.Error())
			return
		}
		nowTime := time.Now()
		// time.Since will return negative number!
		// See: https://stackoverflow.com/questions/50970900/why-is-time-since-returning-negative-durations-on-windows
		if nowTime.Sub(oldTime).Seconds() < 60 {
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		} else {
			rdb.LPush(ctx, key, nowTime.Format(timeFormat))
			rdb.LTrim(ctx, key, 0, int64(maxRequestPerMinute-1))
		}
	}
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
