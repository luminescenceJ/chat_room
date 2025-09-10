package api

import (
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"

	"chatroom/services"
)

// RegisterRoutes 注册API路由
func RegisterRoutes(r *gin.Engine, db *gorm.DB, rdb *redis.Client, wsManager *services.WebSocketManager) {
	// 创建服务
	userService := services.NewUserService(db, rdb)
	messageService := services.NewMessageService(db, rdb)
	groupService := services.NewGroupService(db)
	kafkaService := wsManager.GetKafkaService()

	// 创建控制器
	authController := NewAuthController(userService)
	userController := NewUserController(userService)
	messageController := NewMessageController(messageService, userService)
	groupController := NewGroupController(groupService)
	wsController := NewWebSocketController(db, rdb, userService, wsManager)
	monitorController := NewMonitorController(wsManager, kafkaService)

	// 公开路由
	public := r.Group("/api")
	{
		// 认证相关
		public.POST("/register", authController.Register)
		public.POST("/login", authController.Login)
	}

	// 需要认证的路由
	api := r.Group("/api")
	{
		// 用户相关
		api.GET("/users", userController.GetAllUsers)
		api.GET("/users/:id", userController.GetUserByID)
		api.PUT("/users/:id", userController.UpdateUser)
		api.GET("/users/online", wsController.GetOnlineUsers)

		// 消息相关
		api.GET("/messages", messageController.GetMessages)
		api.POST("/messages", messageController.SendMessage)
		api.GET("/messages/:id", messageController.GetMessage)

		// 群组相关
		api.GET("/groups", groupController.GetGroups)
		api.POST("/groups", groupController.CreateGroup)
		api.GET("/groups/:id", groupController.GetGroupByID)
		api.PUT("/groups/:id", groupController.UpdateGroup)
		api.DELETE("/groups/:id", groupController.DeleteGroup)
		api.POST("/groups/:id/members", groupController.AddMember)
		api.DELETE("/groups/:id/members/:userId", groupController.RemoveMember)

		// WebSocket
		api.GET("/ws", wsController.HandleWebSocket)

		// 监控相关
		api.GET("/monitor/system", monitorController.GetSystemStatus)
		api.GET("/monitor/connections", monitorController.GetConnectionStats)
	}
}
