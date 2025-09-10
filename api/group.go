package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"chatroom/models"
	"chatroom/services"
)

// GroupController 群组控制器
type GroupController struct {
	GroupService *services.GroupService
}

// NewGroupController 创建群组控制器
func NewGroupController(groupService *services.GroupService) *GroupController {
	return &GroupController{
		GroupService: groupService,
	}
}

// CreateGroup 创建群组
func (c *GroupController) CreateGroup(ctx *gin.Context) {
	// 从上下文中获取用户ID
	userID, exists := ctx.Get("userID")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
		return
	}

	var req models.GroupRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误: " + err.Error()})
		return
	}

	// 创建群组
	group, err := c.GroupService.CreateGroup(userID.(uint), req.Name, req.Description, req.Avatar)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取群组响应
	groupResp, err := c.GroupService.GetGroupResponse(group.ID, true)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "群组创建成功",
		"group":   groupResp,
	})
}

// GetGroupByID 根据ID获取群组
func (c *GroupController) GetGroupByID(ctx *gin.Context) {
	// 获取群组ID参数
	groupIDStr := ctx.Param("id")
	groupID, err := strconv.ParseUint(groupIDStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "无效的群组ID"})
		return
	}

	// 是否包含成员信息
	includeMembers := ctx.DefaultQuery("include_members", "false") == "true"

	// 获取群组信息
	groupResp, err := c.GroupService.GetGroupResponse(uint(groupID), includeMembers)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"group": groupResp,
	})
}

// GetUserGroups 获取用户加入的群组
func (c *GroupController) GetUserGroups(ctx *gin.Context) {
	// 从上下文中获取用户ID
	userID, exists := ctx.Get("userID")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
		return
	}

	// 获取用户群组
	groups, err := c.GroupService.GetUserGroups(userID.(uint))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"groups": groups,
	})
}

// UpdateGroup 更新群组信息
func (c *GroupController) UpdateGroup(ctx *gin.Context) {
	// 从上下文中获取用户ID
	userID, exists := ctx.Get("userID")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
		return
	}

	// 获取群组ID参数
	groupIDStr := ctx.Param("id")
	groupID, err := strconv.ParseUint(groupIDStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "无效的群组ID"})
		return
	}

	var req models.GroupRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误: " + err.Error()})
		return
	}

	// 更新群组
	group, err := c.GroupService.UpdateGroup(uint(groupID), userID.(uint), req.Name, req.Description, req.Avatar)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取群组响应
	groupResp, err := c.GroupService.GetGroupResponse(group.ID, false)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "群组更新成功",
		"group":   groupResp,
	})
}

// JoinGroup 加入群组
func (c *GroupController) JoinGroup(ctx *gin.Context) {
	// 从上下文中获取用户ID
	userID, exists := ctx.Get("userID")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
		return
	}

	// 获取群组ID参数
	groupIDStr := ctx.Param("id")
	groupID, err := strconv.ParseUint(groupIDStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "无效的群组ID"})
		return
	}

	// 加入群组
	err = c.GroupService.JoinGroup(uint(groupID), userID.(uint))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "成功加入群组",
	})
}

// LeaveGroup 离开群组
func (c *GroupController) LeaveGroup(ctx *gin.Context) {
	// 从上下文中获取用户ID
	userID, exists := ctx.Get("userID")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
		return
	}

	// 获取群组ID参数
	groupIDStr := ctx.Param("id")
	groupID, err := strconv.ParseUint(groupIDStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "无效的群组ID"})
		return
	}

	// 离开群组
	err = c.GroupService.LeaveGroup(uint(groupID), userID.(uint))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "成功离开群组",
	})
}

// SetGroupAdmin 设置群组管理员
func (c *GroupController) SetGroupAdmin(ctx *gin.Context) {
	// 从上下文中获取用户ID
	userID, exists := ctx.Get("userID")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
		return
	}

	// 获取群组ID参数
	groupIDStr := ctx.Param("id")
	groupID, err := strconv.ParseUint(groupIDStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "无效的群组ID"})
		return
	}

	var req struct {
		UserID  uint `json:"user_id" binding:"required"`
		IsAdmin bool `json:"is_admin"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误: " + err.Error()})
		return
	}

	// 设置管理员
	err = c.GroupService.SetGroupAdmin(uint(groupID), userID.(uint), req.UserID, req.IsAdmin)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "设置管理员成功",
	})
}

// DisbandGroup 解散群组
func (c *GroupController) DisbandGroup(ctx *gin.Context) {
	// 从上下文中获取用户ID
	userID, exists := ctx.Get("userID")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
		return
	}

	// 获取群组ID参数
	groupIDStr := ctx.Param("id")
	groupID, err := strconv.ParseUint(groupIDStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "无效的群组ID"})
		return
	}

	// 解散群组
	err = c.GroupService.DisbandGroup(uint(groupID), userID.(uint))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "群组解散成功",
	})
}

// GetGroupMembers 获取群组成员
func (c *GroupController) GetGroupMembers(ctx *gin.Context) {
	// 获取群组ID参数
	groupIDStr := ctx.Param("id")
	groupID, err := strconv.ParseUint(groupIDStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "无效的群组ID"})
		return
	}

	// 获取群组成员
	members, err := c.GroupService.GetGroupMembers(uint(groupID))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"members": members,
	})
}

// GetGroups 获取群组列表
func (c *GroupController) GetGroups(ctx *gin.Context) {
	// 从上下文中获取用户ID
	userID, exists := ctx.Get("userID")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
		return
	}

	// 获取用户群组
	groups, err := c.GroupService.GetUserGroups(userID.(uint))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"groups": groups,
	})
}

// DeleteGroup 删除群组
func (c *GroupController) DeleteGroup(ctx *gin.Context) {
	// 从上下文中获取用户ID
	userID, exists := ctx.Get("userID")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
		return
	}

	// 获取群组ID参数
	groupIDStr := ctx.Param("id")
	groupID, err := strconv.ParseUint(groupIDStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "无效的群组ID"})
		return
	}

	// 删除群组（实际上是解散群组）
	err = c.GroupService.DisbandGroup(uint(groupID), userID.(uint))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "群组删除成功",
	})
}

// AddMember 添加群组成员
func (c *GroupController) AddMember(ctx *gin.Context) {
	// 从上下文中获取用户ID
	userID, exists := ctx.Get("userID")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
		return
	}

	// 获取群组ID参数
	groupIDStr := ctx.Param("id")
	groupID, err := strconv.ParseUint(groupIDStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "无效的群组ID"})
		return
	}

	var req struct {
		UserID uint `json:"user_id" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误: " + err.Error()})
		return
	}

	// 添加成员（需要检查权限）
	err = c.GroupService.AddMember(uint(groupID), userID.(uint), req.UserID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "成员添加成功",
	})
}

// RemoveMember 移除群组成员
func (c *GroupController) RemoveMember(ctx *gin.Context) {
	// 从上下文中获取用户ID
	userID, exists := ctx.Get("userID")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
		return
	}

	// 获取群组ID参数
	groupIDStr := ctx.Param("id")
	groupID, err := strconv.ParseUint(groupIDStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "无效的群组ID"})
		return
	}

	// 获取要移除的用户ID
	targetUserIDStr := ctx.Param("userId")
	targetUserID, err := strconv.ParseUint(targetUserIDStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "无效的用户ID"})
		return
	}

	// 移除成员（需要检查权限）
	err = c.GroupService.RemoveMember(uint(groupID), userID.(uint), uint(targetUserID))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "成员移除成功",
	})
}
