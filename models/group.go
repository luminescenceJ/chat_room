package models

import (
	"time"
)

// Group 群组模型
type Group struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"not null"`
	Description string    `json:"description"`
	Avatar      string    `json:"avatar"`
	CreatorID   uint      `json:"creator_id" gorm:"not null"`
	Creator     User      `json:"creator" gorm:"foreignKey:CreatorID"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Members     []User    `json:"members,omitempty" gorm:"many2many:group_members;"`
}

// GroupMember 群组成员关联表
type GroupMember struct {
	GroupID   uint      `gorm:"primaryKey"`
	UserID    uint      `gorm:"primaryKey"`
	JoinedAt  time.Time `json:"joined_at"`
	IsAdmin   bool      `json:"is_admin" gorm:"default:false"`
}

// GroupResponse 群组响应模型
type GroupResponse struct {
	ID          uint           `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Avatar      string         `json:"avatar"`
	CreatorID   uint           `json:"creator_id"`
	CreatedAt   time.Time      `json:"created_at"`
	MemberCount int            `json:"member_count"`
	Members     []UserResponse `json:"members,omitempty"`
}

// GroupRequest 创建/更新群组请求模型
type GroupRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Avatar      string `json:"avatar"`
}