package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"chatroom/models"
	"chatroom/services"
)

// MessageController 消息控制器
type MessageController struct {
	MessageService *services.MessageService
	UserService    *services.UserService
}

// NewMessageController 创建消息控制器
func NewMessageController(messageService *services.MessageService, userService *services.UserService) *MessageController {
	return &MessageController{
		MessageService: messageService,
		UserService:    userService,
	}
}

// SendMessage 发送消息
func (c *MessageController) SendMessage(ctx *gin.Context) {
	// 从上下文中获取用户ID
	userID, exists := ctx.Get("userID")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
		return
	}

	var req models.MessageRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误: " + err.Error()})
		return
	}

	// 创建消息
	msg := &models.Message{
		Content:    req.Content,
		Type:       req.Type,
		SenderID:   userID.(uint),
		ReceiverID: req.ReceiverID,
		GroupID:    req.GroupID,
		CreatedAt:  time.Now(),
	}

	// 处理消息
	err := c.MessageService.ProcessMessage(msg)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "消息发送成功",
		"msg_id":  msg.ID,
	})
}

// GetPrivateMessages 获取私聊消息
func (c *MessageController) GetPrivateMessages(ctx *gin.Context) {
	// 从上下文中获取用户ID
	userID, exists := ctx.Get("userID")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
		return
	}

	// 获取对方用户ID
	otherUserIDStr := ctx.Param("user_id")
	otherUserID, err := strconv.ParseUint(otherUserIDStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "无效的用户ID"})
		return
	}

	// 获取分页参数
	limitStr := ctx.DefaultQuery("limit", "20")
	offsetStr := ctx.DefaultQuery("offset", "0")
	limit, _ := strconv.Atoi(limitStr)
	offset, _ := strconv.Atoi(offsetStr)

	// 获取消息
	messages, err := c.MessageService.GetMessagesByUser(userID.(uint), uint(otherUserID), limit, offset)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"messages": messages,
	})
}

// GetGroupMessages 获取群聊消息
func (c *MessageController) GetGroupMessages(ctx *gin.Context) {
	// 从上下文中获取用户ID
	_, exists := ctx.Get("userID")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
		return
	}

	// 获取群组ID
	groupIDStr := ctx.Param("group_id")
	groupID, err := strconv.ParseUint(groupIDStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "无效的群组ID"})
		return
	}

	// 获取分页参数
	limitStr := ctx.DefaultQuery("limit", "20")
	offsetStr := ctx.DefaultQuery("offset", "0")
	limit, _ := strconv.Atoi(limitStr)
	offset, _ := strconv.Atoi(offsetStr)

	// 获取消息
	messages, err := c.MessageService.GetGroupMessages(uint(groupID), limit, offset)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"messages": messages,
	})
}

// GetRecentChats 获取最近的聊天列表
func (c *MessageController) GetRecentChats(ctx *gin.Context) {
	// 从上下文中获取用户ID
	userID, exists := ctx.Get("userID")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
		return
	}

	// 获取最近聊天
	chats, err := c.MessageService.GetRecentChats(userID.(uint))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"chats": chats,
	})
}

// MarkAsRead 标记消息为已读
func (c *MessageController) MarkAsRead(ctx *gin.Context) {
	// 从上下文中获取用户ID
	userID, exists := ctx.Get("userID")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
		return
	}

	var req struct {
		TargetID uint `json:"target_id" binding:"required"` // 对方用户ID或群组ID
		IsGroup  bool `json:"is_group"`                     // 是否为群组
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误: " + err.Error()})
		return
	}

	// 标记为已读
	err := c.MessageService.MarkMessagesAsRead(userID.(uint), req.TargetID, req.IsGroup)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "标记已读成功",
	})
}

// GetMessages 获取消息列表（通用方法）
func (c *MessageController) GetMessages(ctx *gin.Context) {
	// 从上下文中获取用户ID
	userID, exists := ctx.Get("userID")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
		return
	}

	// 获取查询参数
	chatType := ctx.Query("type")      // private 或 group
	targetID := ctx.Query("target_id") // 对方用户ID或群组ID

	if chatType == "" || targetID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "缺少必要参数"})
		return
	}

	targetIDUint, err := strconv.ParseUint(targetID, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "无效的目标ID"})
		return
	}

	// 获取分页参数
	limitStr := ctx.DefaultQuery("limit", "20")
	offsetStr := ctx.DefaultQuery("offset", "0")
	limit, _ := strconv.Atoi(limitStr)
	offset, _ := strconv.Atoi(offsetStr)

	var messages []models.MessageResponse
	if chatType == "private" {
		messages, err = c.MessageService.GetMessagesByUser(userID.(uint), uint(targetIDUint), limit, offset)
	} else if chatType == "group" {
		messages, err = c.MessageService.GetGroupMessages(uint(targetIDUint), limit, offset)
	} else {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "无效的聊天类型"})
		return
	}

	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"messages": messages,
	})
}

// GetMessage 获取单个消息（暂时返回空实现）
func (c *MessageController) GetMessage(ctx *gin.Context) {
	messageID := ctx.Param("id")
	ctx.JSON(http.StatusOK, gin.H{
		"message_id": messageID,
		"message":    "获取单个消息功能待实现",
	})
}
