package services

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"chatroom/models"
)

// GroupService 群组服务
type GroupService struct {
	DB *gorm.DB
}

// NewGroupService 创建群组服务实例
func NewGroupService(db *gorm.DB) *GroupService {
	return &GroupService{DB: db}
}

// CreateGroup 创建新群组
func (s *GroupService) CreateGroup(creatorID uint, name, description, avatar string) (*models.Group, error) {
	// 检查群组名是否已存在
	var existingGroup models.Group
	if err := s.DB.Where("name = ?", name).First(&existingGroup).Error; err == nil {
		return nil, errors.New("群组名已存在")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	// 创建新群组
	group := &models.Group{
		Name:        name,
		Description: description,
		Avatar:      avatar,
		CreatorID:   creatorID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// 开启事务
	tx := s.DB.Begin()
	if err := tx.Create(group).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// 创建者自动加入群组并成为管理员
	groupMember := models.GroupMember{
		GroupID:  group.ID,
		UserID:   creatorID,
		JoinedAt: time.Now(),
		IsAdmin:  true,
	}

	if err := tx.Create(&groupMember).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// 提交事务
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	return group, nil
}

// GetGroupByID 根据ID获取群组
func (s *GroupService) GetGroupByID(id uint) (*models.Group, error) {
	var group models.Group
	if err := s.DB.First(&group, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("群组不存在")
		}
		return nil, err
	}
	return &group, nil
}

// GetGroupResponse 获取群组响应模型
func (s *GroupService) GetGroupResponse(id uint, includeMembers bool) (*models.GroupResponse, error) {
	group, err := s.GetGroupByID(id)
	if err != nil {
		return nil, err
	}

	// 获取成员数量
	var memberCount int64
	if err := s.DB.Model(&models.GroupMember{}).Where("group_id = ?", id).Count(&memberCount).Error; err != nil {
		return nil, err
	}

	response := &models.GroupResponse{
		ID:          group.ID,
		Name:        group.Name,
		Description: group.Description,
		Avatar:      group.Avatar,
		CreatorID:   group.CreatorID,
		CreatedAt:   group.CreatedAt,
		MemberCount: int(memberCount),
	}

	// 如果需要包含成员信息
	if includeMembers {
		var members []models.User
		if err := s.DB.Table("users").
			Joins("JOIN group_members ON users.id = group_members.user_id").
			Where("group_members.group_id = ?", id).
			Find(&members).Error; err != nil {
			return nil, err
		}

		// 获取在线用户ID集合
		onlineUserIDs := make(map[uint]bool)
		GlobalHub.mu.RLock()
		for id := range GlobalHub.clients {
			onlineUserIDs[id] = true
		}
		GlobalHub.mu.RUnlock()

		// 构建成员响应
		memberResponses := make([]models.UserResponse, len(members))
		for i, member := range members {
			memberResponses[i] = models.UserResponse{
				ID:       member.ID,
				Username: member.Username,
				Email:    member.Email,
				Avatar:   member.Avatar,
				Online:   onlineUserIDs[member.ID],
			}
		}

		response.Members = memberResponses
	}

	return response, nil
}

// GetUserGroups 获取用户加入的所有群组
func (s *GroupService) GetUserGroups(userID uint) ([]models.GroupResponse, error) {
	var groupIDs []uint
	if err := s.DB.Table("group_members").
		Select("group_id").
		Where("user_id = ?", userID).
		Pluck("group_id", &groupIDs).Error; err != nil {
		return nil, err
	}

	var groups []models.Group
	if err := s.DB.Where("id IN ?", groupIDs).Find(&groups).Error; err != nil {
		return nil, err
	}

	// 获取每个群组的成员数量
	groupMemberCounts := make(map[uint]int64)
	for _, groupID := range groupIDs {
		var count int64
		if err := s.DB.Model(&models.GroupMember{}).Where("group_id = ?", groupID).Count(&count).Error; err != nil {
			return nil, err
		}
		groupMemberCounts[groupID] = count
	}

	// 构建响应
	responses := make([]models.GroupResponse, len(groups))
	for i, group := range groups {
		responses[i] = models.GroupResponse{
			ID:          group.ID,
			Name:        group.Name,
			Description: group.Description,
			Avatar:      group.Avatar,
			CreatorID:   group.CreatorID,
			CreatedAt:   group.CreatedAt,
			MemberCount: int(groupMemberCounts[group.ID]),
		}
	}

	return responses, nil
}

// UpdateGroup 更新群组信息
func (s *GroupService) UpdateGroup(id, userID uint, name, description, avatar string) (*models.Group, error) {
	// 检查群组是否存在
	group, err := s.GetGroupByID(id)
	if err != nil {
		return nil, err
	}

	// 检查用户是否有权限更新群组（创建者或管理员）
	var isAdmin bool
	err = s.DB.Model(&models.GroupMember{}).
		Select("is_admin").
		Where("group_id = ? AND user_id = ?", id, userID).
		First(&isAdmin).Error

	if err != nil || !isAdmin {
		return nil, errors.New("没有权限更新群组")
	}

	// 检查群组名是否已被其他群组使用
	if name != group.Name {
		var existingGroup models.Group
		if err := s.DB.Where("name = ? AND id != ?", name, id).First(&existingGroup).Error; err == nil {
			return nil, errors.New("群组名已存在")
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		group.Name = name
	}

	// 更新其他信息
	group.Description = description
	if avatar != "" {
		group.Avatar = avatar
	}
	group.UpdatedAt = time.Now()

	// 保存到数据库
	if err := s.DB.Save(group).Error; err != nil {
		return nil, err
	}

	return group, nil
}

// JoinGroup 加入群组
func (s *GroupService) JoinGroup(groupID, userID uint) error {
	// 检查群组是否存在
	if _, err := s.GetGroupByID(groupID); err != nil {
		return err
	}

	// 检查用户是否已在群组中
	var count int64
	if err := s.DB.Model(&models.GroupMember{}).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		Count(&count).Error; err != nil {
		return err
	}

	if count > 0 {
		return errors.New("已经是群组成员")
	}

	// 加入群组
	groupMember := models.GroupMember{
		GroupID:  groupID,
		UserID:   userID,
		JoinedAt: time.Now(),
		IsAdmin:  false,
	}

	if err := s.DB.Create(&groupMember).Error; err != nil {
		return err
	}

	return nil
}

// LeaveGroup 离开群组
func (s *GroupService) LeaveGroup(groupID, userID uint) error {
	// 检查群组是否存在
	group, err := s.GetGroupByID(groupID)
	if err != nil {
		return err
	}

	// 创建者不能离开群组
	if group.CreatorID == userID {
		return errors.New("群组创建者不能离开群组")
	}

	// 检查用户是否在群组中
	var count int64
	if err := s.DB.Model(&models.GroupMember{}).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		Count(&count).Error; err != nil {
		return err
	}

	if count == 0 {
		return errors.New("不是群组成员")
	}

	// 离开群组
	if err := s.DB.Where("group_id = ? AND user_id = ?", groupID, userID).Delete(&models.GroupMember{}).Error; err != nil {
		return err
	}

	return nil
}

// SetGroupAdmin 设置群组管理员
func (s *GroupService) SetGroupAdmin(groupID, userID, targetUserID uint, isAdmin bool) error {
	// 检查群组是否存在
	group, err := s.GetGroupByID(groupID)
	if err != nil {
		return err
	}

	// 只有创建者可以设置管理员
	if group.CreatorID != userID {
		return errors.New("没有权限设置管理员")
	}

	// 检查目标用户是否在群组中
	var count int64
	if err := s.DB.Model(&models.GroupMember{}).
		Where("group_id = ? AND user_id = ?", groupID, targetUserID).
		Count(&count).Error; err != nil {
		return err
	}

	if count == 0 {
		return errors.New("目标用户不是群组成员")
	}

	// 更新管理员状态
	if err := s.DB.Model(&models.GroupMember{}).
		Where("group_id = ? AND user_id = ?", groupID, targetUserID).
		Update("is_admin", isAdmin).Error; err != nil {
		return err
	}

	return nil
}

// DisbandGroup 解散群组
func (s *GroupService) DisbandGroup(groupID, userID uint) error {
	// 检查群组是否存在
	group, err := s.GetGroupByID(groupID)
	if err != nil {
		return err
	}

	// 只有创建者可以解散群组
	if group.CreatorID != userID {
		return errors.New("没有权限解散群组")
	}

	// 开启事务
	tx := s.DB.Begin()

	// 删除所有群组成员
	if err := tx.Where("group_id = ?", groupID).Delete(&models.GroupMember{}).Error; err != nil {
		tx.Rollback()
		return err
	}

	// 删除群组
	if err := tx.Delete(&models.Group{}, groupID).Error; err != nil {
		tx.Rollback()
		return err
	}

	// 提交事务
	if err := tx.Commit().Error; err != nil {
		return err
	}

	return nil
}

// GetGroupMembers 获取群组成员
func (s *GroupService) GetGroupMembers(groupID uint) ([]models.UserResponse, error) {
	var members []models.User
	if err := s.DB.Table("users").
		Joins("JOIN group_members ON users.id = group_members.user_id").
		Where("group_members.group_id = ?", groupID).
		Find(&members).Error; err != nil {
		return nil, err
	}

	// 获取在线用户ID集合
	onlineUserIDs := make(map[uint]bool)
	GlobalHub.mu.RLock()
	for id := range GlobalHub.clients {
		onlineUserIDs[id] = true
	}
	GlobalHub.mu.RUnlock()

	// 获取管理员信息
	adminMap := make(map[uint]bool)
	var admins []struct {
		UserID uint
	}
	if err := s.DB.Table("group_members").
		Select("user_id").
		Where("group_id = ? AND is_admin = ?", groupID, true).
		Find(&admins).Error; err != nil {
		return nil, err
	}

	for _, admin := range admins {
		adminMap[admin.UserID] = true
	}

	// 构建响应
	responses := make([]models.UserResponse, len(members))
	for i, member := range members {
		responses[i] = models.UserResponse{
			ID:       member.ID,
			Username: member.Username,
			Email:    member.Email,
			Avatar:   member.Avatar,
			Online:   onlineUserIDs[member.ID],
		}
	}

	return responses, nil
}