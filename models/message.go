package models

import (
	"time"
)

// MessageType 消息类型
type MessageType string

const (
	PrivateMessage MessageType = "private" // 私聊消息
	GroupMessage   MessageType = "group"   // 群聊消息
	SystemMessage  MessageType = "system"  // 系统消息
)

// Message 消息模型
type Message struct {
	ID         uint        `json:"id" gorm:"primaryKey"`
	Content    string      `json:"content" gorm:"not null"`
	Type       MessageType `json:"type" gorm:"not null"`
	SenderID   uint        `json:"sender_id" gorm:"not null"`
	Sender     User        `json:"sender" gorm:"foreignKey:SenderID"`
	ReceiverID uint        `json:"receiver_id"`        // 接收者ID（用户ID或群组ID）
	GroupID    uint        `json:"group_id,omitempty"` // 群组ID，私聊时为0
	CreatedAt  time.Time   `json:"created_at"`
}

// MessageRequest 消息请求模型
type MessageRequest struct {
	Content    string      `json:"content" binding:"required"`
	Type       MessageType `json:"type" binding:"required"`
	ReceiverID uint        `json:"receiver_id" binding:"required"`
	GroupID    uint        `json:"group_id,omitempty"`
}

// MessageResponse 消息响应模型
type MessageResponse struct {
	ID         uint         `json:"id"`
	Content    string       `json:"content"`
	Type       MessageType  `json:"type"`
	SenderID   uint         `json:"sender_id"`
	Sender     UserResponse `json:"sender"`
	ReceiverID uint         `json:"receiver_id,omitempty"`
	GroupID    uint         `json:"group_id,omitempty"`
	CreatedAt  time.Time    `json:"created_at"`
}

// RecentChat 最近聊天模型
type RecentChat struct {
	TargetID      uint      `json:"target_id"`
	Type          string    `json:"type"` // "private" or "group"
	Name          string    `json:"name"`
	Avatar        string    `json:"avatar"`
	LastMessage   string    `json:"last_message"`
	LastMessageAt time.Time `json:"last_message_at"`
	UnreadCount   int       `json:"unread_count"`
	Online        bool      `json:"online,omitempty"` // For private chats
}
