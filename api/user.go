package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"chatroom/models"
	"chatroom/services"
)

// UserController 用户控制器
type UserController struct {
	UserService *services.UserService
}

// NewUserController 创建用户控制器
func NewUserController(userService *services.UserService) *UserController {
	return &UserController{
		UserService: userService,
	}
}

// GetAllUsers 获取所有用户
func (c *UserController) GetAllUsers(ctx *gin.Context) {
	users, err := c.UserService.GetAllUsers()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"users": users,
	})
}

// GetUserByID 根据ID获取用户
func (c *UserController) GetUserByID(ctx *gin.Context) {
	// 获取用户ID参数
	userIDStr := ctx.Param("id")
	userID, err := strconv.ParseUint(userIDStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "无效的用户ID"})
		return
	}

	// 获取用户信息
	userResp, err := c.UserService.GetUserResponse(uint(userID))
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"user": userResp,
	})
}

// GetOnlineUsers 获取在线用户
func (c *UserController) GetOnlineUsers(ctx *gin.Context) {
	onlineUsers := services.GlobalHub.GetOnlineUsers()
	ctx.JSON(http.StatusOK, gin.H{
		"online_users": onlineUsers,
	})
}

// SearchUsers 搜索用户
func (c *UserController) SearchUsers(ctx *gin.Context) {
	// 获取查询参数
	query := ctx.Query("q")
	if query == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "查询参数不能为空"})
		return
	}

	// 这里应该调用用户服务的搜索方法
	// 简化处理，返回所有用户
	users, err := c.UserService.GetAllUsers()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 过滤用户
	var filteredUsers []models.UserResponse
	for _, user := range users {
		// 简单的字符串匹配
		if contains(user.Username, query) || contains(user.Email, query) {
			filteredUsers = append(filteredUsers, user)
		}
	}

	ctx.JSON(http.StatusOK, gin.H{
		"users": filteredUsers,
	})
}

// contains 检查字符串是否包含子串（不区分大小写）
func contains(s, substr string) bool {
	s, substr = strings.ToLower(s), strings.ToLower(substr)
	return strings.Contains(s, substr)
}