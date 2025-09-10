package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"chatroom/api"
	"chatroom/config"
	"chatroom/middleware"
	"chatroom/models"
	"chatroom/services"
)

func main() {
	// 设置最大处理器数量
	runtime.GOMAXPROCS(runtime.NumCPU())

	// 加载配置
	config.LoadConfig()

	// 连接数据库
	dsn := config.AppConfig.DBConnectionString
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger:      logger.Default.LogMode(logger.Silent),
		PrepareStmt: true, // 缓存预编译语句
	})
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}

	// 配置数据库连接池
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("获取数据库连接池失败: %v", err)
	}
	sqlDB.SetMaxIdleConns(config.AppConfig.DBMaxIdleConns)
	sqlDB.SetMaxOpenConns(config.AppConfig.DBMaxOpenConns)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// 自动迁移数据库表结构
	err = db.AutoMigrate(&models.User{}, &models.Message{}, &models.Group{}, &models.GroupMember{})
	if err != nil {
		log.Fatalf("数据库迁移失败: %v", err)
	}

	// 初始化Redis客户端（仅用于缓存）
	rdb := redis.NewClient(&redis.Options{
		Addr:     config.AppConfig.RedisAddr,
		Password: config.AppConfig.RedisPassword,
		DB:       config.AppConfig.RedisDB,
		PoolSize: config.AppConfig.RedisPoolSize,
	})

	// 测试Redis连接
	ctx := context.Background()
	_, err = rdb.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("Redis连接失败: %v", err)
	}
	log.Println("Redis连接成功")

	// 初始化用户服务
	userService := services.NewUserService(db, rdb)

	// 初始化Kafka服务（允许失败）
	kafkaService, err := services.NewKafkaService()
	if err != nil {
		log.Printf("警告: Kafka服务初始化失败: %v", err)
		log.Println("应用将在没有Kafka的情况下运行（消息不会通过队列分发）")
		kafkaService = nil
	}

	// 初始化消息服务
	messageService := services.NewMessageService(db, rdb, userService, kafkaService)

	// 初始化WebSocket管理器
	wsManager := services.NewWebSocketManager(rdb, messageService, userService)
	go wsManager.Run()

	// 创建Gin实例
	if config.AppConfig.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.Default()

	// 配置CORS
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	// 添加限流中间件
	r.Use(middleware.RateLimiter(rdb))

	// 使用JWT中间件
	r.Use(middleware.JWTAuth())

	// 注册路由
	api.RegisterRoutes(r, db, rdb, wsManager)

	// 优雅关闭
	srv := services.StartServer(r, config.AppConfig.Port)

	// 等待中断信号以优雅地关闭服务器
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("正在关闭服务器...")

	// 停止WebSocket管理器（会关闭Kafka连接）
	wsManager.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("服务器强制关闭:", err)
	}

	log.Println("服务器已优雅关闭")
}
