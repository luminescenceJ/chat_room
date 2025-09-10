package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"

	"chatroom/models"
)

// MessageService 处理消息的存储和检索
type MessageService struct {
	db          *gorm.DB
	rdb         *redis.Client
	userService *UserService
	kafka       *KafkaService
}

// NewMessageService 创建一个新的消息服务
func NewMessageService(db *gorm.DB, rdb *redis.Client, userService *UserService, kafka *KafkaService) *MessageService {
	return &MessageService{
		db:          db,
		rdb:         rdb,
		userService: userService,
		kafka:       kafka,
	}
}

// ProcessMessage 处理并分发消息
func (s *MessageService) ProcessMessage(msg *models.Message) error {
	// 1. 保存消息到数据库
	if err := s.SaveMessage(msg); err != nil {
		return err
	}

	// 2. 获取发送者信息
	sender, err := s.userService.GetUserResponse(msg.SenderID)
	if err != nil {
		return err
	}

	// 3. 构建消息响应
	msgResp := models.MessageResponse{
		ID:         msg.ID,
		Content:    msg.Content,
		Type:       msg.Type,
		SenderID:   msg.SenderID,
		Sender:     *sender,
		ReceiverID: msg.ReceiverID,
		GroupID:    msg.GroupID,
		CreatedAt:  msg.CreatedAt,
	}

	msgJSON, _ := json.Marshal(msgResp)

	// 4. 推送到Kafka（如果可用）
	if s.kafka != nil {
		var topic string
		if msg.GroupID > 0 { // 群聊消息
			topic = s.kafka.BuildTopicName("group", msg.GroupID)
		} else { // 私聊消息
			topic = s.kafka.BuildTopicName("private", msg.ReceiverID)
		}

		if err := s.kafka.PublishMessage(topic, "message", msgJSON); err != nil {
			log.Printf("发布消息到Kafka失败: %v", err)
			// 非致命错误，消息已保存
		}
	} else {
		log.Printf("Kafka不可用，跳过消息发布")
	}

	// 5. 更新最近聊天列表和缓存
	s.updateRecentChats(msg)
	s.cacheRecentMessage(&msgResp)

	return nil
}

// SaveMessage 保存消息到数据库
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

	return nil
}

// GetMessagesByUser 获取两个用户之间的消息
func (s *MessageService) GetMessagesByUser(userID1, userID2 uint, limit, offset int) ([]models.MessageResponse, error) {
	var messages []models.Message
	err := s.db.Preload("Sender").
		Where("(sender_id = ? AND receiver_id = ?) OR (sender_id = ? AND receiver_id = ?)", userID1, userID2, userID2, userID1).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&messages).Error

	if err != nil {
		return nil, err
	}

	return s.convertMessagesToResponse(messages)
}

// GetGroupMessages 获取群组消息
func (s *MessageService) GetGroupMessages(groupID uint, limit, offset int) ([]models.MessageResponse, error) {
	var messages []models.Message
	err := s.db.Preload("Sender").
		Where("group_id = ?", groupID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&messages).Error

	if err != nil {
		return nil, err
	}

	return s.convertMessagesToResponse(messages)
}

// GetGroupMembers 获取群组成员ID列表
func (s *MessageService) GetGroupMembers(groupID uint) ([]uint, error) {
	var members []models.GroupMember

	// 先尝试从Redis缓存获取
	ctx := context.Background()
	groupKey := fmt.Sprintf("group:members:%d", groupID)

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
		key = fmt.Sprintf("recent:group:%d", groupID)
	} else {
		key = fmt.Sprintf("recent:private:%d", receiverID)
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
			ID:       msg.ID,
			Content:  msg.Content,
			Type:     msg.Type,
			SenderID: msg.SenderID,
			Sender: models.UserResponse{
				ID:       msg.Sender.ID,
				Username: msg.Sender.Username,
				Avatar:   msg.Sender.Avatar,
				Online:   s.userService.IsUserOnline(msg.Sender.ID),
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
func (s *MessageService) cacheRecentMessage(msgResp *models.MessageResponse) {
	ctx := context.Background()
	msgJSON, _ := json.Marshal(msgResp)

	var key string
	if msgResp.GroupID > 0 {
		key = fmt.Sprintf("recent:group:%d", msgResp.GroupID)
	} else {
		// 私聊消息，需要给收发双方都缓存
		key = fmt.Sprintf("recent:private:%d:%d", msgResp.SenderID, msgResp.ReceiverID)
		key2 := fmt.Sprintf("recent:private:%d:%d", msgResp.ReceiverID, msgResp.SenderID)
		s.rdb.LPush(ctx, key2, msgJSON)
		s.rdb.LTrim(ctx, key2, 0, 99)
	}

	s.rdb.LPush(ctx, key, msgJSON)
	s.rdb.LTrim(ctx, key, 0, 99) // 保留最近100条
}

// GetRecentChats 获取最近的聊天列表
func (s *MessageService) GetRecentChats(userID uint) ([]models.RecentChat, error) {
	ctx := context.Background()
	key := fmt.Sprintf("recent:chats:%d", userID)

	// 尝试从缓存获取
	cachedData, err := s.rdb.Get(ctx, key).Result()
	if err == nil {
		var chats []models.RecentChat
		if json.Unmarshal([]byte(cachedData), &chats) == nil {
			return chats, nil
		}
	}

	// 缓存未命中，从数据库查询
	// 1. 获取用户加入的所有群组
	var userGroups []models.GroupMember
	s.db.Where("user_id = ?", userID).Find(&userGroups)

	// 2. 获取与用户相关的私聊
	var privateMessages []models.Message
	s.db.Where("sender_id = ? OR receiver_id = ?", userID, userID).
		Order("created_at DESC").
		Limit(1000). // 限制查询范围
		Find(&privateMessages)

	chatMap := make(map[string]models.RecentChat)

	// 处理群聊
	for _, ug := range userGroups {
		var lastMsg models.Message
		res := s.db.Where("group_id = ?", ug.GroupID).Order("created_at DESC").First(&lastMsg)
		if res.Error == nil {
			var group models.Group
			s.db.First(&group, ug.GroupID)
			chatKey := fmt.Sprintf("group-%d", ug.GroupID)
			chatMap[chatKey] = models.RecentChat{
				TargetID:      ug.GroupID,
				Type:          "group",
				Name:          group.Name,
				Avatar:        group.Avatar,
				LastMessage:   lastMsg.Content,
				LastMessageAt: lastMsg.CreatedAt,
				UnreadCount:   s.getUnreadCount(userID, ug.GroupID, true),
			}
		}
	}

	// 处理私聊
	for _, msg := range privateMessages {
		otherUserID := msg.SenderID
		if msg.SenderID == userID {
			otherUserID = msg.ReceiverID
		}
		if otherUserID == userID {
			continue
		}

		chatKey := fmt.Sprintf("private-%d", otherUserID)
		if existingChat, ok := chatMap[chatKey]; !ok || msg.CreatedAt.After(existingChat.LastMessageAt) {
			user, err := s.userService.GetUserByID(otherUserID)
			if err != nil {
				continue
			}
			chatMap[chatKey] = models.RecentChat{
				TargetID:      otherUserID,
				Type:          "private",
				Name:          user.Username,
				Avatar:        user.Avatar,
				LastMessage:   msg.Content,
				LastMessageAt: msg.CreatedAt,
				UnreadCount:   s.getUnreadCount(userID, otherUserID, false),
				Online:        s.userService.IsUserOnline(otherUserID),
			}
		}
	}

	var chats []models.RecentChat
	for _, chat := range chatMap {
		chats = append(chats, chat)
	}

	// 按最后消息时间排序
	sort.Slice(chats, func(i, j int) bool {
		return chats[i].LastMessageAt.After(chats[j].LastMessageAt)
	})

	// 缓存结果
	jsonData, _ := json.Marshal(chats)
	s.rdb.Set(ctx, key, jsonData, 5*time.Minute)

	return chats, nil
}

// MarkMessagesAsRead 标记消息为已读
func (s *MessageService) MarkMessagesAsRead(userID, targetID uint, isGroup bool) error {
	ctx := context.Background()
	var unreadKey string
	if isGroup {
		unreadKey = fmt.Sprintf("unread:%d:group:%d", userID, targetID)
	} else {
		unreadKey = fmt.Sprintf("unread:%d:private:%d", userID, targetID)
	}
	return s.rdb.Del(ctx, unreadKey).Err()
}

// updateRecentChats 更新用户的最近聊天列表
func (s *MessageService) updateRecentChats(msg *models.Message) {
	ctx := context.Background()
	if msg.GroupID > 0 {
		// 群聊：更新所有成员的最近聊天列表
		memberIDs, err := s.GetGroupMembers(msg.GroupID)
		if err != nil {
			return
		}
		for _, memberID := range memberIDs {
			s.rdb.Del(ctx, fmt.Sprintf("recent:chats:%d", memberID))
			if memberID != msg.SenderID {
				s.incrementUnreadCount(memberID, msg.GroupID, true)
			}
		}
	} else {
		// 私聊：更新收发双方的最近聊天列表
		s.rdb.Del(ctx, fmt.Sprintf("recent:chats:%d", msg.SenderID))
		s.rdb.Del(ctx, fmt.Sprintf("recent:chats:%d", msg.ReceiverID))
		s.incrementUnreadCount(msg.ReceiverID, msg.SenderID, false)
	}
}

func (s *MessageService) incrementUnreadCount(userID, targetID uint, isGroup bool) {
	ctx := context.Background()
	var unreadKey string
	if isGroup {
		unreadKey = fmt.Sprintf("unread:%d:group:%d", userID, targetID)
	} else {
		unreadKey = fmt.Sprintf("unread:%d:private:%d", userID, targetID)
	}
	s.rdb.Incr(ctx, unreadKey)
}

func (s *MessageService) getUnreadCount(userID, targetID uint, isGroup bool) int {
	ctx := context.Background()
	var unreadKey string
	if isGroup {
		unreadKey = fmt.Sprintf("unread:%d:group:%d", userID, targetID)
	} else {
		unreadKey = fmt.Sprintf("unread:%d:private:%d", userID, targetID)
	}
	count, _ := s.rdb.Get(ctx, unreadKey).Int()
	return count
}

func (s *MessageService) convertMessagesToResponse(messages []models.Message) ([]models.MessageResponse, error) {
	responses := make([]models.MessageResponse, len(messages))
	for i, msg := range messages {
		sender, err := s.userService.GetUserResponse(msg.SenderID)
		if err != nil {
			// 如果获取发送者失败，可以跳过或使用默认值
			sender = &models.UserResponse{ID: msg.SenderID, Username: "未知用户"}
		}
		responses[i] = models.MessageResponse{
			ID:         msg.ID,
			Content:    msg.Content,
			Type:       msg.Type,
			SenderID:   msg.SenderID,
			Sender:     *sender,
			ReceiverID: msg.ReceiverID,
			GroupID:    msg.GroupID,
			CreatedAt:  msg.CreatedAt,
		}
	}
	// 反转消息顺序，使之按时间升序
	for i, j := 0, len(responses)-1; i < j; i, j = i+1, j-1 {
		responses[i], responses[j] = responses[j], responses[i]
	}
	return responses, nil
}
