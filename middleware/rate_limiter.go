package middleware

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

// RateLimiter 创建一个基于Redis的限流中间件
func RateLimiter(rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取客户端IP
		clientIP := c.ClientIP()

		// 对于WebSocket连接，使用更宽松的限制
		if c.Request.URL.Path == "/ws" {
			// WebSocket连接每分钟限制5次
			key := "rate_limit:ws:" + clientIP
			handleRateLimit(c, rdb, key, 5, 60*time.Second)
			return
		}

		// 普通API请求每分钟限制60次
		key := "rate_limit:api:" + clientIP
		handleRateLimit(c, rdb, key, 60, 60*time.Second)
	}
}

// handleRateLimit 处理限流逻辑
func handleRateLimit(c *gin.Context, rdb *redis.Client, key string, limit int, duration time.Duration) {
	ctx := context.Background()

	// 获取当前计数
	count, err := rdb.Get(ctx, key).Int()
	if err == redis.Nil {
		// 键不存在，设置初始值
		rdb.Set(ctx, key, 1, duration)
		count = 1
	} else if err != nil {
		// 发生错误，允许请求通过
		c.Next()
		return
	} else {
		// 键存在，增加计数

		count = int(rdb.Incr(ctx, key).Val())
	}

	// 设置响应头
	c.Header("X-RateLimit-Limit", strconv.Itoa(limit))
	c.Header("X-RateLimit-Remaining", strconv.Itoa(limit-count))

	// 检查是否超过限制
	if count > limit {
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
			"error": "请求过于频繁，请稍后再试",
		})
		return
	}

	c.Next()
}
