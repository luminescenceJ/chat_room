package services

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"

	"chatroom/models"
)

// MessageService 处理消息的存储和检索
type MessageService struct {
	db  *gorm.DB
	rdb *redis.Client
}

// NewMessageService 创建一个新的消息服务
func NewMessageService(db *gorm.DB, rdb *redis.Client) *MessageService {
	return &MessageService{
		db:  db,
		rdb: rdb,
	}
}

// SaveMessage 保存消息到数据库和缓存
func (s *MessageService) SaveMessage(msg *models.Message) error {
	// 使用事务保存消息
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(msg).Error; err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		log.Printf("保存消息失败: %v", err)
		return err
	}

	// 缓存最近的消息
	s.cacheRecentMessage(msg)

	return nil
}

// GetUserByID 根据ID获取用户信息
func (s *MessageService) GetUserByID(userID uint) (*models.User, error) {
	var user models.User
	
	// 先尝试从Redis缓存获取
	ctx := context.Background()
	userKey := "user:" + string(userID)
	
	userJSON, err := s.rdb.Get(ctx, userKey).Result()
	if err == nil {
		// 缓存命中
		err = json.Unmarshal([]byte(userJSON), &user)
		if err == nil {
			return &user, nil
		}
	}
	
	// 缓存未命中，从数据库获取
	if err := s.db.First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("用户不存在")
		}
		return nil, err
	}
	
	// 更新缓存
	userBytes, _ := json.Marshal(user)
	s.rdb.Set(ctx, userKey, userBytes, 30*time.Minute)
	
	return &user, nil
}

// GetGroupMembers 获取群组成员ID列表
func (s *MessageService) GetGroupMembers(groupID uint) ([]uint, error) {
	var members []models.GroupMember
	
	// 先尝试从Redis缓存获取
	ctx := context.Background()
	groupKey := "group:members:" + string(groupID)
	
	membersJSON, err := s.rdb.Get(ctx, groupKey).Result()
	if err == nil {
		// 缓存命中
		var memberIDs []uint
		err = json.Unmarshal([]byte(membersJSON), &memberIDs)
		if err == nil {
			return memberIDs, nil
		}
	}
	
	// 缓存未命中，从数据库获取
	if err := s.db.Where("group_id = ?", groupID).Find(&members).Error; err != nil {
		return nil, err
	}
	
	memberIDs := make([]uint, len(members))
	for i, member := range members {
		memberIDs[i] = member.UserID
	}
	
	// 更新缓存
	memberBytes, _ := json.Marshal(memberIDs)
	s.rdb.Set(ctx, groupKey, memberBytes, 5*time.Minute)
	
	return memberIDs, nil
}

// GetRecentMessages 获取最近的消息
func (s *MessageService) GetRecentMessages(receiverID, groupID uint, limit int) ([]models.MessageResponse, error) {
	var key string
	
	if groupID > 0 {
		key = "recent:group:" + string(groupID)
	} else {
		key = "recent:private:" + string(receiverID)
	}
	
	ctx := context.Background()
	
	// 尝试从缓存获取
	messagesJSON, err := s.rdb.LRange(ctx, key, 0, int64(limit-1)).Result()
	if err == nil && len(messagesJSON) > 0 {
		messages := make([]models.MessageResponse, 0, len(messagesJSON))
		
		for _, msgJSON := range messagesJSON {
			var msg models.MessageResponse
			if err := json.Unmarshal([]byte(msgJSON), &msg); err == nil {
				messages = append(messages, msg)
			}
		}
		
		return messages, nil
	}
	
	// 缓存未命中，从数据库获取
	var messages []models.Message
	query := s.db.Preload("Sender")
	
	if groupID > 0 {
		query = query.Where("group_id = ?", groupID)
	} else {
		query = query.Where("(sender_id = ? AND receiver_id = ?) OR (sender_id = ? AND receiver_id = ?)", 
			receiverID, receiverID, receiverID, receiverID)
	}
	
	if err := query.Order("created_at DESC").Limit(limit).Find(&messages).Error; err != nil {
		return nil, err
	}
	
	// 转换为响应格式
	responses := make([]models.MessageResponse, len(messages))
	for i, msg := range messages {
		responses[i] = models.MessageResponse{
			ID:         msg.ID,
			Content:    msg.Content,
			Type:       msg.Type,
			SenderID:   msg.SenderID,
			Sender: models.UserResponse{
				ID:       msg.Sender.ID,
				Username: msg.Sender.Username,
				Online:   true, // 这里需要检查用户是否在线
			},
			ReceiverID: msg.ReceiverID,
			GroupID:    msg.GroupID,
			CreatedAt:  msg.CreatedAt,
		}
		
		// 更新缓存
		msgJSON, _ := json.Marshal(responses[i])
		s.rdb.RPush(ctx, key, msgJSON)
	}
	
	// 设置缓存过期时间
	s.rdb.Expire(ctx, key, 10*time.Minute)
	
	return responses, nil
}

// cacheRecentMessage 缓存最近的消息
func (s *MessageService) cacheRecentMessage(msg *models.Message) {
	ctx := context.Background()
	
	// 获取发送者信息
	sender, err := s.GetUserByID(msg.SenderID)
	if err != nil {
		log.Printf("获取发送者信息失败: %v", err)
		return
	}
	
	// 创建消息响应
	msgResp := models.MessageResponse{
		ID:         msg.ID,
		Content:    msg.Content,
		Type:       msg.Type,
		SenderID:   msg.SenderID,
		Sender: models.UserResponse{
			ID:       sender.ID,
			Username: sender.Username,
			Online:   true,
		},
		ReceiverID: msg.ReceiverID,
		GroupID:    msg.GroupID,
		CreatedAt:  msg.CreatedAt,
	}
	
	// 序列化消息
	msgJSON, _ := json.Marshal(msgResp)
	
	// 缓存到相应的列表
	var key string
	if msg.GroupID > 0 {
		key = "recent:group:" + string(msg.GroupID)
	} else {
		key = "recent:private:" + string(msg.ReceiverID)
		// 同时缓存到发送者的最近消息
		senderKey := "recent:private:" + string(msg.SenderID)
		s.rdb.LPush(ctx, senderKey, msgJSON)
		s.rdb.LTrim(ctx, senderKey, 0, 99) // 保留最近100条
		s.rdb.Expire(ctx, senderKey, 10*time.Minute)
	}
	
	s.rdb.LPush(ctx, key, msgJSON)
	s.rdb.LTrim(ctx, key, 0, 99) // 保留最近100条
	s.rdb.Expire(ctx, key, 10*time.Minute)
}