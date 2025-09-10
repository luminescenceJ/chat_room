package services

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"chatroom/models"
)

// Upgrader WebSocket升级器
var Upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有跨域请求
	},
}

// Client 表示一个WebSocket客户端
type Client struct {
	ID       uint
	Username string
	Conn     *websocket.Conn
	Send     chan []byte
}

// WritePump 将消息从通道发送到WebSocket连接
func (c *Client) WritePump() {
	ticker := time.NewTicker(30 * time.Second)
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

// ReadPump 从WebSocket连接读取消息
func (c *Client) ReadPump(wsManager *WebSocketManager, messageService *MessageService) {
	defer func() {
		wsManager.UnregisterClient(c)
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
		go c.handleReceivedMessage(message, wsManager, messageService)
	}
}

// handleReceivedMessage 处理接收到的消息
func (c *Client) handleReceivedMessage(message []byte, wsManager *WebSocketManager, messageService *MessageService) {
	var wsMsg WebSocketMessage
	if err := json.Unmarshal(message, &wsMsg); err != nil {
		log.Printf("解析消息失败: %v", err)
		return
	}
	
	ctx := context.Background()
	
	switch wsMsg.Type {
	case "chat_message":
		var msgReq models.MessageRequest
		if err := json.Unmarshal(wsMsg.Content, &msgReq); err != nil {
			log.Printf("解析聊天消息失败: %v", err)
			return
		}
		
		// 处理消息（保存到数据库并转发）
		c.handleChatMessage(ctx, msgReq, wsManager, messageService)
		
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
		c.handleTypingNotification(ctx, typingData.ReceiverID, typingData.GroupID, wsManager)
		
	default:
		log.Printf("未知消息类型: %s", wsMsg.Type)
	}
}

// handleChatMessage 处理聊天消息
func (c *Client) handleChatMessage(ctx context.Context, msgReq models.MessageRequest, wsManager *WebSocketManager, messageService *MessageService) {
	// 保存消息到数据库
	msg := &models.Message{
		Content:    msgReq.Content,
		Type:       msgReq.Type,
		SenderID:   c.ID,
		ReceiverID: msgReq.ReceiverID,
		GroupID:    msgReq.GroupID,
		CreatedAt:  time.Now(),
	}
	
	// 异步保存消息到数据库
	go func() {
		if err := messageService.SaveMessage(msg); err != nil {
			log.Printf("保存消息失败: %v", err)
		}
	}()
	
	// 获取发送者信息
	sender, err := messageService.GetUserByID(c.ID)
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
	
	// 根据消息类型发送到Kafka
	if msg.Type == models.PrivateMessage {
		// 发布到Kafka私聊主题
		wsManager.PublishMessage(ctx, "chat_message", msgJSON, msg.ReceiverID, 0)
	} else if msg.Type == models.GroupMessage {
		// 发布到Kafka群组主题
		wsManager.PublishMessage(ctx, "chat_message", msgJSON, 0, msg.GroupID)
	}
}

// handleTypingNotification 处理typing通知
func (c *Client) handleTypingNotification(ctx context.Context, receiverID, groupID uint, wsManager *WebSocketManager) {
	typingData := struct {
		SenderID   uint   `json:"sender_id"`
		Username   string `json:"username"`
		ReceiverID uint   `json:"receiver_id,omitempty"`
		GroupID    uint   `json:"group_id,omitempty"`
	}{
		SenderID:   c.ID,
		Username:   c.Username,
		ReceiverID: receiverID,
		GroupID:    groupID,
	}
	
	typingJSON, _ := json.Marshal(typingData)
	
	if groupID > 0 {
		// 发布到Kafka群组主题
		wsManager.PublishMessage(ctx, "typing", typingJSON, 0, groupID)
	} else {
		// 发布到Kafka私聊主题
		wsManager.PublishMessage(ctx, "typing", typingJSON, receiverID, 0)
	}
}
