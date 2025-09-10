package services

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"chatroom/config"
	"chatroom/models"
)

// Message 表示一个WebSocket消息
type WebSocketMessage struct {
	Type      string          `json:"type"`
	Content   json.RawMessage `json:"content"`
	Timestamp time.Time       `json:"timestamp"`
}

// Hub 维护活跃的客户端集合并广播消息
type Hub struct {
	// 注册的客户端
	clients map[uint]*Client

	// 注册请求
	register chan *Client

	// 注销请求
	unregister chan *Client

	// 广播消息通道
	broadcast chan []byte

	// 互斥锁保护clients map
	mu sync.RWMutex
}

// 创建一个新的Hub
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[uint]*Client),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan []byte, config.AppConfig.ChannelBuffSize),
		mu:         sync.RWMutex{},
	}
}

// 运行Hub
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client.ID] = client
			h.mu.Unlock()
			log.Printf("客户端已连接: %s (ID: %d)", client.Username, client.ID)

			// 广播用户上线消息
			h.broadcastUserStatus(client.ID, client.Username, true)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client.ID]; ok {
				delete(h.clients, client.ID)
				close(client.Send)
				log.Printf("客户端已断开连接: %s (ID: %d)", client.Username, client.ID)

				// 广播用户下线消息
				h.broadcastUserStatus(client.ID, client.Username, false)
			}
			h.mu.Unlock()

		case message := <-h.broadcast:
			// 广播消息给所有客户端
			h.mu.RLock()
			for _, client := range h.clients {
				select {
				case client.Send <- message:
				default:
					close(client.Send)
					h.mu.RUnlock()
					h.mu.Lock()
					delete(h.clients, client.ID)
					h.mu.Unlock()
					h.mu.RLock()
				}
			}
			h.mu.RUnlock()
		}
	}
}

// 广播用户状态变更
func (h *Hub) broadcastUserStatus(userID uint, username string, online bool) {
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
	h.broadcast <- msgJSON
}

// 发送消息给特定用户
func (h *Hub) SendToUser(userID uint, message []byte) bool {
	h.mu.RLock()
	client, exists := h.clients[userID]
	h.mu.RUnlock()

	if exists {
		client.Send <- message
		return true
	}
	return false
}

// 发送消息给群组成员
func (h *Hub) SendToGroup(groupID uint, userIDs []uint, message []byte) {
	h.mu.RLock()
	for _, userID := range userIDs {
		if client, exists := h.clients[userID]; exists {
			client.Send <- message
		}
	}
	h.mu.RUnlock()
}

// 获取在线用户列表
func (h *Hub) GetOnlineUsers() []models.UserResponse {
	h.mu.RLock()
	defer h.mu.RUnlock()

	onlineUsers := make([]models.UserResponse, 0, len(h.clients))
	for id, client := range h.clients {
		onlineUsers = append(onlineUsers, models.UserResponse{
			ID:       id,
			Username: client.Username,
			Online:   true,
		})
	}

	return onlineUsers
}

// 全局Hub实例
var GlobalHub = NewHub()

// WebSocket升级器
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有跨域请求
	},
}

// HandleWebSocket 处理WebSocket连接
func HandleWebSocket(c *gin.Context, userID uint, username string) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("升级WebSocket连接失败:", err)
		return
	}

	client := &Client{
		ID:       userID,
		Username: username,
		Conn:     conn,
		Send:     make(chan []byte, 256),
	}

	GlobalHub.register <- client

	// 允许收集垃圾
	go client.writePump()
	go client.readPump()
}

// writePump 将消息从通道发送到WebSocket连接
func (c *Client) writePump() {
	ticker := time.NewTicker(60 * time.Second)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				// 通道已关闭
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// 添加队列中的消息
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.Send)
			}

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// readPump 从WebSocket连接读取消息
func (c *Client) readPump() {
	defer func() {
		GlobalHub.unregister <- c
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(512 * 1024) // 512KB
	c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("错误: %v", err)
			}
			break
		}

		// 处理接收到的消息
		go handleReceivedMessage(c.ID, message)
	}
}

// 处理接收到的消息
func handleReceivedMessage(senderID uint, message []byte) {
	var wsMsg WebSocketMessage
	if err := json.Unmarshal(message, &wsMsg); err != nil {
		log.Printf("解析消息失败: %v", err)
		return
	}

	switch wsMsg.Type {
	case "chat_message":
		var msgReq models.MessageRequest
		if err := json.Unmarshal(wsMsg.Content, &msgReq); err != nil {
			log.Printf("解析聊天消息失败: %v", err)
			return
		}

		// 处理消息（保存到数据库并转发）
		handleChatMessage(senderID, msgReq)

	case "typing":
		var typingData struct {
			ReceiverID uint `json:"receiver_id"`
			GroupID    uint `json:"group_id,omitempty"`
		}
		if err := json.Unmarshal(wsMsg.Content, &typingData); err != nil {
			log.Printf("解析typing消息失败: %v", err)
			return
		}

		// 处理typing通知
		handleTypingNotification(senderID, typingData.ReceiverID, typingData.GroupID)

	default:
		log.Printf("未知消息类型: %s", wsMsg.Type)
	}
}

// 处理聊天消息
func handleChatMessage(senderID uint, msgReq models.MessageRequest) {
	// 保存消息到数据库
	msg := models.Message{
		Content:    msgReq.Content,
		Type:       msgReq.Type,
		SenderID:   senderID,
		ReceiverID: msgReq.ReceiverID,
		GroupID:    msgReq.GroupID,
		CreatedAt:  time.Now(),
	}

	// 这里应该调用数据库服务保存消息
	// dbService.SaveMessage(msg)

	// 获取发送者信息
	// sender := dbService.GetUserByID(senderID)
	sender := models.UserResponse{
		ID:       senderID,
		Username: "用户" + fmt.Sprintf("%d", senderID), // 临时使用，实际应从数据库获取
		Online:   true,
	}

	// 创建消息响应
	msgResp := models.MessageResponse{
		ID:         msg.ID,
		Content:    msg.Content,
		Type:       msg.Type,
		SenderID:   msg.SenderID,
		Sender:     sender,
		ReceiverID: msg.ReceiverID,
		GroupID:    msg.GroupID,
		CreatedAt:  msg.CreatedAt,
	}

	// 序列化消息
	msgJSON, _ := json.Marshal(msgResp)

	wsMsg := WebSocketMessage{
		Type:      "chat_message",
		Content:   msgJSON,
		Timestamp: time.Now(),
	}

	wsMsgJSON, _ := json.Marshal(wsMsg)

	// 根据消息类型发送
	if msg.Type == models.PrivateMessage {
		// 发送给接收者
		GlobalHub.SendToUser(msg.ReceiverID, wsMsgJSON)
		// 发送给发送者（回显）
		GlobalHub.SendToUser(msg.SenderID, wsMsgJSON)
	} else if msg.Type == models.GroupMessage {
		// 获取群组成员
		// members := dbService.GetGroupMembers(msg.GroupID)
		// GlobalHub.SendToGroup(msg.GroupID, memberIDs, wsMsgJSON)

		// 临时方案：广播给所有人
		GlobalHub.broadcast <- wsMsgJSON
	}
}

// 处理typing通知
func handleTypingNotification(senderID, receiverID, groupID uint) {
	typingData := struct {
		SenderID   uint `json:"sender_id"`
		ReceiverID uint `json:"receiver_id,omitempty"`
		GroupID    uint `json:"group_id,omitempty"`
	}{
		SenderID:   senderID,
		ReceiverID: receiverID,
		GroupID:    groupID,
	}

	typingJSON, _ := json.Marshal(typingData)

	wsMsg := WebSocketMessage{
		Type:      "typing",
		Content:   typingJSON,
		Timestamp: time.Now(),
	}

	wsMsgJSON, _ := json.Marshal(wsMsg)

	if groupID > 0 {
		// 获取群组成员
		// members := dbService.GetGroupMembers(groupID)
		// GlobalHub.SendToGroup(groupID, memberIDs, wsMsgJSON)

		// 临时方案：广播给所有人
		GlobalHub.broadcast <- wsMsgJSON
	} else {
		// 私聊typing通知
		GlobalHub.SendToUser(receiverID, wsMsgJSON)
	}
}
