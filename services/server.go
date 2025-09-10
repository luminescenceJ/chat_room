package services

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// StartServer 启动HTTP服务器
func StartServer(r *gin.Engine, port string) *http.Server {
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// 在后台启动服务器
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("监听失败: %s\n", err)
		}
	}()

	log.Println("服务器启动在端口:", port)
	return srv
}
