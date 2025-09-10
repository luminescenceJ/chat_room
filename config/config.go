package config

import (
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// AppConfig 应用配置
var AppConfig struct {
	// 服务器配置
	Port               string
	Mode               string // debug 或 release
	JWTSecret          string
	MaxConnections     int    // 最大WebSocket连接数
	
	// Redis配置（仅用于缓存）
	RedisAddr          string
	RedisPassword      string
	RedisDB            int
	RedisPoolSize      int
	
	// Kafka配置（用于消息队列）
	KafkaBootstrapServers []string
	KafkaConsumerGroup    string
	KafkaTopicPrefix      string
	KafkaPartitions       int
	KafkaReplicationFactor int
	
	// 数据库配置
	DBConnectionString string
	DBMaxIdleConns     int
	DBMaxOpenConns     int
	
	// 缓存配置
	CacheExpiration    int // 缓存过期时间（秒）
	
	// 消息队列配置
	ChannelBuffSize    int
}

// LoadConfig 从环境变量加载配置
func LoadConfig() {
	// 尝试加载.env文件
	err := godotenv.Load()
	if err != nil {
		log.Println("未找到.env文件，将使用环境变量")
	}

	// 服务器配置
	AppConfig.Port = getEnv("PORT", "8080")
	AppConfig.Mode = getEnv("MODE", "debug")
	AppConfig.JWTSecret = getEnv("JWT_SECRET", "your-secret-key")
	
	maxConn, err := strconv.Atoi(getEnv("MAX_CONNECTIONS", "10000"))
	if err != nil {
		maxConn = 10000
	}
	AppConfig.MaxConnections = maxConn
	
	// Redis配置
	AppConfig.RedisAddr = getEnv("REDIS_ADDR", "localhost:6379")
	AppConfig.RedisPassword = getEnv("REDIS_PASSWORD", "")
	
	redisDB, err := strconv.Atoi(getEnv("REDIS_DB", "0"))
	if err != nil {
		redisDB = 0
	}
	AppConfig.RedisDB = redisDB
	
	redisPoolSize, err := strconv.Atoi(getEnv("REDIS_POOL_SIZE", strconv.Itoa(runtime.NumCPU() * 10)))
	if err != nil {
		redisPoolSize = runtime.NumCPU() * 10
	}
	AppConfig.RedisPoolSize = redisPoolSize
	
	// Kafka配置
	kafkaServers := getEnv("KAFKA_BOOTSTRAP_SERVERS", "localhost:9092")
	AppConfig.KafkaBootstrapServers = strings.Split(kafkaServers, ",")
	AppConfig.KafkaConsumerGroup = getEnv("KAFKA_CONSUMER_GROUP", "chatroom-group")
	AppConfig.KafkaTopicPrefix = getEnv("KAFKA_TOPIC_PREFIX", "chatroom-")
	
	kafkaPartitions, err := strconv.Atoi(getEnv("KAFKA_PARTITIONS", "3"))
	if err != nil {
		kafkaPartitions = 3
	}
	AppConfig.KafkaPartitions = kafkaPartitions
	
	kafkaReplication, err := strconv.Atoi(getEnv("KAFKA_REPLICATION_FACTOR", "2"))
	if err != nil {
		kafkaReplication = 2
	}
	AppConfig.KafkaReplicationFactor = kafkaReplication
	
	// 数据库配置
	AppConfig.DBConnectionString = getEnv("DB_CONNECTION_STRING", "root:password@tcp(127.0.0.1:3306)/chatroom?charset=utf8mb4&parseTime=True&loc=Local")
	
	dbMaxIdleConns, err := strconv.Atoi(getEnv("DB_MAX_IDLE_CONNS", "10"))
	if err != nil {
		dbMaxIdleConns = 10
	}
	AppConfig.DBMaxIdleConns = dbMaxIdleConns
	
	dbMaxOpenConns, err := strconv.Atoi(getEnv("DB_MAX_OPEN_CONNS", "100"))
	if err != nil {
		dbMaxOpenConns = 100
	}
	AppConfig.DBMaxOpenConns = dbMaxOpenConns
	
	// 缓存配置
	cacheExpiration, err := strconv.Atoi(getEnv("CACHE_EXPIRATION", "300"))
	if err != nil {
		cacheExpiration = 300
	}
	AppConfig.CacheExpiration = cacheExpiration
	
	// 消息队列配置
	channelBuff, err := strconv.Atoi(getEnv("CHANNEL_BUFFER_SIZE", "1000"))
	if err != nil {
		channelBuff = 1000
	}
	AppConfig.ChannelBuffSize = channelBuff
	
	log.Println("配置加载完成")
}

// getEnv 获取环境变量，如果不存在则返回默认值
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}
