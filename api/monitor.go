package api

import (
	"net/http"
	"runtime"

	"github.com/gin-gonic/gin"

	"chatroom/services"
)

// MonitorController 监控控制器
type MonitorController struct {
	WSManager   *services.WebSocketManager
	KafkaService *services.KafkaService
}

// NewMonitorController 创建监控控制器
func NewMonitorController(wsManager *services.WebSocketManager, kafkaService *services.KafkaService) *MonitorController {
	return &MonitorController{
		WSManager:    wsManager,
		KafkaService: kafkaService,
	}
}

// GetSystemStatus 获取系统状态
func (c *MonitorController) GetSystemStatus(ctx *gin.Context) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// 获取Kafka指标
	kafkaMetrics := c.KafkaService.GetMetrics()

	ctx.JSON(http.StatusOK, gin.H{
		"connections": c.WSManager.GetConnectionCount(),
		"goroutines":  runtime.NumGoroutine(),
		"memory": gin.H{
			"alloc":      m.Alloc / 1024 / 1024,      // MB
			"total_alloc": m.TotalAlloc / 1024 / 1024, // MB
			"sys":        m.Sys / 1024 / 1024,        // MB
			"num_gc":     m.NumGC,
		},
		"kafka": gin.H{
			"messages_sent":     kafkaMetrics["messages_sent"],
			"messages_received": kafkaMetrics["messages_received"],
			"errors":            kafkaMetrics["errors"],
		},
	})
}

// GetConnectionStats 获取连接统计信息
func (c *MonitorController) GetConnectionStats(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{
		"connections": c.WSManager.GetConnectionCount(),
	})
}