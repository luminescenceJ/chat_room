package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"

	"chatroom/services"
)

// WebSocketController WebSocket控制器
type WebSocketController struct {
	UserService    *services.UserService
	MessageService *services.MessageService
	WSManager      *services.WebSocketManager
}

// NewWebSocketController 创建WebSocket控制器
func NewWebSocketController(
	db *gorm.DB,
	rdb *redis.Client,
	userService *services.UserService,
	wsManager *services.WebSocketManager,
) *WebSocketController {
	messageService := services.NewMessageService(db, rdb)
	
	return &WebSocketController{
		UserService:    userService,
		MessageService: messageService,
		WSManager:      wsManager,
	}
}

// HandleWebSocket 处理WebSocket连接
func (c *WebSocketController) HandleWebSocket(ctx *gin.Context) {
	// 从上下文中获取用户信息
	userID, exists := ctx.Get("userID")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
		return
	}

	username, exists := ctx.Get("username")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
		return
	}

	// 处理WebSocket连接
	c.handleConnection(ctx, userID.(uint), username.(string))
}

// handleConnection 处理WebSocket连接
func (c *WebSocketController) handleConnection(ctx *gin.Context, userID uint, username string) {
	// 创建WebSocket连接
	conn, err := services.Upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "WebSocket升级失败"})
		return
	}
	
	// 创建客户端
	client := &services.Client{
		ID:       userID,
		Username: username,
		Conn:     conn,
		Send:     make(chan []byte, 256),
	}
	
	// 注册客户端
	if !c.WSManager.RegisterClient(client) {
		conn.Close()
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器已达到最大连接数"})
		return
	}
	
	// 订阅用户私聊频道
	c.WSManager.SubscribeToUserChannel(userID)
	
	// 获取用户所在的群组
	groups, err := c.UserService.GetUserGroups(userID)
	if err == nil {
		// 订阅用户所在的所有群组频道
		for _, group := range groups {
			c.WSManager.SubscribeToGroupChannel(userID, group.ID)
		}
	}
	
	// 启动读写协程
	go client.WritePump()
	go client.ReadPump(c.WSManager, c.MessageService)
}

// GetOnlineUsers 获取在线用户列表
func (c *WebSocketController) GetOnlineUsers(ctx *gin.Context) {
	onlineUsers := c.WSManager.GetOnlineUsers()
	ctx.JSON(http.StatusOK, gin.H{
		"users": onlineUsers,
	})
}

// GetConnectionStats 获取连接统计信息
func (c *WebSocketController) GetConnectionStats(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{
		"connections": c.WSManager.GetConnectionCount(),
	})
}
