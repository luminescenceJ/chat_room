package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"golang.org/x/crypto/bcrypt"
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

// Register 用户注册
func (s *UserService) Register(username, password, email string) (*models.User, error) {
	// 检查用户名或邮箱是否已存在
	var existingUser models.User
	if err := s.db.Where("username = ? OR email = ?", username, email).First(&existingUser).Error; err == nil {
		return nil, errors.New("用户名或邮箱已存在")
	}

	// 哈希密码
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.New("密码加密失败")
	}

	// 创建新用户
	newUser := models.User{
		Username: username,
		Password: string(hashedPassword),
		Email:    email,
		Avatar:   fmt.Sprintf("https://api.multiavatar.com/%s.png", username), // Default avatar
	}

	if err := s.db.Create(&newUser).Error; err != nil {
		return nil, errors.New("用户注册失败")
	}

	return &newUser, nil
}

// Login 用户登录
func (s *UserService) Login(username, password string) (*models.User, error) {
	var user models.User
	if err := s.db.Where("username = ?", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("用户不存在")
		}
		return nil, err
	}

	// 比较密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, errors.New("密码错误")
	}

	return &user, nil
}

// GetAllUsers 获取所有用户
func (s *UserService) GetAllUsers() ([]models.UserResponse, error) {
	var users []models.User
	if err := s.db.Find(&users).Error; err != nil {
		return nil, err
	}

	var userResponses []models.UserResponse
	for _, user := range users {
		userResponses = append(userResponses, models.UserResponse{
			ID:       user.ID,
			Username: user.Username,
			Email:    user.Email,
			Avatar:   user.Avatar,
			Online:   s.IsUserOnline(user.ID), // Check online status
		})
	}
	return userResponses, nil
}

// GetUserResponse 根据ID获取用户响应信息
func (s *UserService) GetUserResponse(id uint) (*models.UserResponse, error) {
	user, err := s.GetUserByID(id)
	if err != nil {
		return nil, err
	}

	return &models.UserResponse{
		ID:       user.ID,
		Username: user.Username,
		Email:    user.Email,
		Avatar:   user.Avatar,
		Online:   s.IsUserOnline(id),
	}, nil
}

// IsUserOnline 检查用户是否在线
func (s *UserService) IsUserOnline(userID uint) bool {
	ctx := context.Background()
	isMember, err := s.rdb.SIsMember(ctx, keyOnlineUsers, fmt.Sprintf("%d", userID)).Result()
	if err != nil {
		return false
	}
	return isMember
}

// UpdateUser 更新用户信息
func (s *UserService) UpdateUser(id uint, username, email, avatar string) (*models.User, error) {
	var user models.User
	if err := s.db.First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("用户不存在")
		}
		return nil, err
	}

	if username != "" {
		user.Username = username
	}
	if email != "" {
		user.Email = email
	}
	if avatar != "" {
		user.Avatar = avatar
	}

	if err := s.db.Save(&user).Error; err != nil {
		return nil, errors.New("更新用户信息失败")
	}

	// 删除缓存
	ctx := context.Background()
	key := fmt.Sprintf("user:%d", id)
	s.rdb.Del(ctx, key)

	return &user, nil
}

// ChangePassword 修改密码
func (s *UserService) ChangePassword(id uint, oldPassword, newPassword string) error {
	var user models.User
	if err := s.db.First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("用户不存在")
		}
		return err
	}

	// 验证旧密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(oldPassword)); err != nil {
		return errors.New("旧密码错误")
	}

	// 哈希新密码
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return errors.New("新密码加密失败")
	}

	user.Password = string(hashedPassword)

	if err := s.db.Save(&user).Error; err != nil {
		return errors.New("修改密码失败")
	}

	return nil
}

// SearchUsers 搜索用户
func (s *UserService) SearchUsers(query string) ([]models.UserResponse, error) {
	var users []models.User
	if err := s.db.Where("username LIKE ? OR email LIKE ?", "%"+query+"%", "%"+query+"%").Find(&users).Error; err != nil {
		return nil, err
	}

	var userResponses []models.UserResponse
	for _, user := range users {
		userResponses = append(userResponses, models.UserResponse{
			ID:       user.ID,
			Username: user.Username,
			Email:    user.Email,
			Avatar:   user.Avatar,
			Online:   s.IsUserOnline(user.ID),
		})
	}
	return userResponses, nil
}

// GetUserByID 根据ID获取用户
func (s *UserService) GetUserByID(id uint) (*models.User, error) {
	var user models.User

	// 先尝试从缓存获取
	ctx := context.Background()
	key := fmt.Sprintf("user:%d", id)

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
	key := fmt.Sprintf("user:groups:%d", userID)

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
		id, err := strconv.ParseUint(idStr, 10, 64)
		if err != nil {
			continue
		}

		user, err := s.GetUserByID(uint(id))
		if err != nil {
			continue
		}

		onlineUsers = append(onlineUsers, models.UserResponse{
			ID:       user.ID,
			Username: user.Username,
			Email:    user.Email,
			Avatar:   user.Avatar,
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
