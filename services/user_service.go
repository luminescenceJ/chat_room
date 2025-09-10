package services

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"

	"chatroom/config"
	"chatroom/models"
)

// UserService 用户服务
type UserService struct {
	db  *gorm.DB
	rdb *redis.Client
}

// NewUserService 创建用户服务
func NewUserService(db *gorm.DB, rdb *redis.Client) *UserService {
	return &UserService{
		db:  db,
		rdb: rdb,
	}
}

// GetUserByID 根据ID获取用户
func (s *UserService) GetUserByID(id uint) (*models.User, error) {
	var user models.User
	
	// 先尝试从缓存获取
	ctx := context.Background()
	key := "user:" + string(id)
	
	userJSON, err := s.rdb.Get(ctx, key).Result()
	if err == nil {
		// 缓存命中
		if err := json.Unmarshal([]byte(userJSON), &user); err == nil {
			return &user, nil
		}
	}
	
	// 从数据库获取
	if err := s.db.First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("用户不存在")
		}
		return nil, err
	}
	
	// 更新缓存
	userBytes, _ := json.Marshal(user)
	s.rdb.Set(ctx, key, userBytes, time.Duration(config.AppConfig.CacheExpiration)*time.Second)
	
	return &user, nil
}

// GetUserGroups 获取用户所在的群组
func (s *UserService) GetUserGroups(userID uint) ([]models.Group, error) {
	var groups []models.Group
	
	// 先尝试从缓存获取
	ctx := context.Background()
	key := "user:groups:" + string(userID)
	
	groupsJSON, err := s.rdb.Get(ctx, key).Result()
	if err == nil {
		// 缓存命中
		if err := json.Unmarshal([]byte(groupsJSON), &groups); err == nil {
			return groups, nil
		}
	}
	
	// 从数据库获取
	if err := s.db.Table("groups").
		Joins("JOIN group_members ON groups.id = group_members.group_id").
		Where("group_members.user_id = ?", userID).
		Find(&groups).Error; err != nil {
		return nil, err
	}
	
	// 更新缓存
	groupsBytes, _ := json.Marshal(groups)
	s.rdb.Set(ctx, key, groupsBytes, time.Duration(config.AppConfig.CacheExpiration)*time.Second)
	
	return groups, nil
}

// GetOnlineUsers 获取在线用户列表
func (s *UserService) GetOnlineUsers() ([]models.UserResponse, error) {
	ctx := context.Background()
	
	// 从Redis获取在线用户ID列表
	userIDs, err := s.rdb.SMembers(ctx, keyOnlineUsers).Result()
	if err != nil {
		return nil, err
	}
	
	onlineUsers := make([]models.UserResponse, 0, len(userIDs))
	
	for _, idStr := range userIDs {
		var id uint
		if err := json.Unmarshal([]byte(idStr), &id); err != nil {
			continue
		}
		
		user, err := s.GetUserByID(id)
		if err != nil {
			continue
		}
		
		onlineUsers = append(onlineUsers, models.UserResponse{
			ID:       user.ID,
			Username: user.Username,
			Online:   true,
		})
	}
	
	return onlineUsers, nil
}

// UpdateUserLastSeen 更新用户最后在线时间
func (s *UserService) UpdateUserLastSeen(userID uint) error {
	return s.db.Model(&models.User{}).Where("id = ?", userID).
		Update("last_seen_at", time.Now()).Error
}