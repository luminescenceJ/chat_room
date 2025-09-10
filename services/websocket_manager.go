package services

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"

	"chatroom/config"
	"chatroom/models"
)

const (
	// Redis键名
	keyOnlineUsers = "chat:online_users"
)

// WebSocketManager 管理WebSocket连接和消息分发
type WebSocketManager struct {
	// 客户端映射表 userID -> client
	clients map[uint]*Client
	
	// 互斥锁保护clients map
	mu sync.RWMutex
	
	// Redis客户端（用于缓存）
	rdb *redis.Client
	
	// Kafka服务（用于消息队列）
	kafka *KafkaService
	
	// 消息服务
	messageService *MessageService
	
	// 连接计数器
	connectionCount int32
	
	// 最大连接数
	maxConnections int32
	
	// 停止信号
	stopCh chan struct{}
}

// NewWebSocketManager 创建一个新的WebSocket管理器
func NewWebSocketManager(rdb *redis.Client, messageService *MessageService) *WebSocketManager {
	// 创建Kafka服务
	kafka, err := NewKafkaService()
	if err != nil {
		log.Fatalf("创建Kafka服务失败: %v", err)
	}
	
	return &WebSocketManager{
		clients:        make(map[uint]*Client),
		mu:             sync.RWMutex{},
		rdb:            rdb,
		kafka:          kafka,
		messageService: messageService,
		maxConnections: int32(config.AppConfig.MaxConnections),
		stopCh:         make(chan struct{}),
	}
}

// Run 启动WebSocket管理器
func (m *WebSocketManager) Run() {
	// 订阅全局消息主题
	err := m.kafka.SubscribeTopic(m.kafka.BuildTopicName("global", 0), func(message []byte) {
		m.broadcastToAll(message)
	})
	
	if err != nil {
		log.Printf("订阅全局消息主题失败: %v", err)
	}
	
	// 订阅用户状态主题
	err = m.kafka.SubscribeTopic(m.kafka.BuildTopicName("status", 0), func(message []byte) {
		m.handleUserStatusUpdate(message)
	})
	
	if err != nil {
		log.Printf("订阅用户状态主题失败: %v", err)
	}
	
	// 定期清理过期的连接
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			m.cleanupExpiredConnections()
		case <-m.stopCh:
			return
		}
	}
}

// Stop 停止WebSocket管理器
func (m *WebSocketManager) Stop() {
	close(m.stopCh)
	m.kafka.Close()
}

// RegisterClient 注册一个新的客户端
func (m *WebSocketManager) RegisterClient(client *Client) bool {
	// 检查连接数是否超过限制
	if atomic.LoadInt32(&m.connectionCount) >= m.maxConnections {
		log.Println("达到最大连接数限制，拒绝新连接")
		return false
	}
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// 如果已存在相同用户ID的连接，先关闭旧连接
	if oldClient, exists := m.clients[client.ID]; exists {
		close(oldClient.Send)
		oldClient.Conn.Close()
	}
	
	m.clients[client.ID] = client
	atomic.AddInt32(&m.connectionCount, 1)
	
	// 将用户添加到在线用户集合
	ctx := context.Background()
	m.rdb.SAdd(ctx, keyOnlineUsers, client.ID)
	
	// 发布用户上线消息
	m.publishUserStatus(client.ID, client.Username, true)
	
	log.Printf("客户端已连接: %s (ID: %d), 当前连接数: %d", client.Username, client.ID, atomic.LoadInt32(&m.connectionCount))
	return true
}

// UnregisterClient 注销一个客户端
func (m *WebSocketManager) UnregisterClient(client *Client) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if _, ok := m.clients[client.ID]; ok {
		delete(m.clients, client.ID)
		close(client.Send)
		atomic.AddInt32(&m.connectionCount, -1)
		
		// 将用户从在线用户集合中移除
		ctx := context.Background()
		m.rdb.SRem(ctx, keyOnlineUsers, client.ID)
		
		// 发布用户下线消息
		m.publishUserStatus(client.ID, client.Username, false)
		
		log.Printf("客户端已断开连接: %s (ID: %d), 当前连接数: %d", client.Username, client.ID, atomic.LoadInt32(&m.connectionCount))
	}
}

// SendToUser 发送消息给特定用户
func (m *WebSocketManager) SendToUser(userID uint, message []byte) bool {
	m.mu.RLock()
	client, exists := m.clients[userID]
	m.mu.RUnlock()
	
	if exists {
		select {
		case client.Send <- message:
			return true
		default:
			// 如果客户端的发送缓冲区已满，关闭连接
			m.mu.Lock()
			delete(m.clients, userID)
			close(client.Send)
			atomic.AddInt32(&m.connectionCount, -1)
			m.mu.Unlock()
			return false
		}
	}
	return false
}

// PublishMessage 发布消息到Kafka
func (m *WebSocketManager) PublishMessage(ctx context.Context, msgType string, message []byte, receiverID, groupID uint) {
	err := m.kafka.PublishChatMessage(msgType, message, receiverID, groupID)
	if err != nil {
		log.Printf("发布消息失败: %v", err)
	}
}

// SubscribeToUserChannel 订阅用户私聊频道
func (m *WebSocketManager) SubscribeToUserChannel(userID uint) {
	topic := m.kafka.BuildTopicName("private", userID)
	
	err := m.kafka.SubscribeTopic(topic, func(message []byte) {
		// 查找用户的客户端连接
		m.mu.RLock()
		client, exists := m.clients[userID]
		m.mu.RUnlock()
		
		if exists {
			select {
			case client.Send <- message:
			default:
				// 如果发送缓冲区已满，关闭连接
				m.mu.Lock()
				delete(m.clients, userID)
				close(client.Send)
				atomic.AddInt32(&m.connectionCount, -1)
				m.mu.Unlock()
			}
		}
	})
	
	if err != nil {
		log.Printf("订阅用户私聊主题失败: %v", err)
	}
}

// SubscribeToGroupChannel 订阅群组频道
func (m *WebSocketManager) SubscribeToGroupChannel(userID, groupID uint) {
	topic := m.kafka.BuildTopicName("group", groupID)
	
	err := m.kafka.SubscribeTopic(topic, func(message []byte) {
		// 查找用户的客户端连接
		m.mu.RLock()
		client, exists := m.clients[userID]
		m.mu.RUnlock()
		
		if exists {
			select {
			case client.Send <- message:
			default:
				// 如果发送缓冲区已满，跳过
			}
		}
	})
	
	if err != nil {
		log.Printf("订阅群组主题失败: %v", err)
	}
}

// broadcastToAll 广播消息给所有连接的客户端
func (m *WebSocketManager) broadcastToAll(message []byte) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	for _, client := range m.clients {
		select {
		case client.Send <- message:
		default:
			// 如果客户端的发送缓冲区已满，跳过
			continue
		}
	}
}

// publishUserStatus 发布用户状态变更消息
func (m *WebSocketManager) publishUserStatus(userID uint, username string, online bool) {
	status := "online"
	if !online {
		status = "offline"
	}
	
	statusMsg := struct {
		UserID   uint   `json:"user_id"`
		Username string `json:"username"`
		Status   string `json:"status"`
	}{
		UserID:   userID,
		Username: username,
		Status:   status,
	}
	
	statusJSON, _ := json.Marshal(statusMsg)
	
	wsMsg := WebSocketMessage{
		Type:      "user_status",
		Content:   statusJSON,
		Timestamp: time.Now(),
	}
	
	msgJSON, _ := json.Marshal(wsMsg)
	
	// 发布到Kafka
	err := m.kafka.PublishMessage(m.kafka.BuildTopicName("status", 0), "", msgJSON)
	if err != nil {
		log.Printf("发布用户状态消息失败: %v", err)
	}
}

// handleUserStatusUpdate 处理用户状态更新消息
func (m *WebSocketManager) handleUserStatusUpdate(message []byte) {
	m.broadcastToAll(message)
}

// GetOnlineUsers 获取在线用户列表
func (m *WebSocketManager) GetOnlineUsers() []models.UserResponse {
	ctx := context.Background()
	userIDs, err := m.rdb.SMembers(ctx, keyOnlineUsers).Result()
	if err != nil {
		log.Printf("获取在线用户失败: %v", err)
		return []models.UserResponse{}
	}
	
	onlineUsers := make([]models.UserResponse, 0, len(userIDs))
	for _, idStr := range userIDs {
		var id uint
		json.Unmarshal([]byte(idStr), &id)
		
		// 从数据库获取用户信息
		user, err := m.messageService.GetUserByID(id)
		if err != nil {
			continue
		}
		
		onlineUsers = append(onlineUsers, models.UserResponse{
			ID:       user.ID,
			Username: user.Username,
			Online:   true,
		})
	}
	
	return onlineUsers
}

// cleanupExpiredConnections 清理过期的连接
func (m *WebSocketManager) cleanupExpiredConnections() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	for userID, client := range m.clients {
		// 检查连接是否已关闭
		if err := client.Conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(time.Second)); err != nil {
			log.Printf("检测到过期连接: %d, 错误: %v", userID, err)
			delete(m.clients, userID)
			close(client.Send)
			atomic.AddInt32(&m.connectionCount, -1)
			
			// 将用户从在线用户集合中移除
			ctx := context.Background()
			m.rdb.SRem(ctx, keyOnlineUsers, userID)
		}
	}
}

// GetConnectionCount 获取当前连接数
func (m *WebSocketManager) GetConnectionCount() int32 {
	return atomic.LoadInt32(&m.connectionCount)
}

// GetKafkaService 获取Kafka服务实例
func (m *WebSocketManager) GetKafkaService() *KafkaService {
	return m.kafka
}
