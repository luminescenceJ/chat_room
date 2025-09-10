package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"chatroom/config"

	"github.com/IBM/sarama"
)

// KafkaService Kafka消息服务
type KafkaService struct {
	producer      sarama.SyncProducer
	asyncProducer sarama.AsyncProducer // 添加异步生产者
	consumer      sarama.ConsumerGroup
	topics        map[string]bool
	topicsMutex   sync.RWMutex
	handlers      map[string]MessageHandler
	handlerMutex  sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	errorChan     chan *sarama.ConsumerError // 添加错误通道
	metrics       *KafkaMetrics              // 添加指标收集
}

// KafkaMetrics 收集Kafka相关指标
type KafkaMetrics struct {
	messagesSent     int64
	messagesReceived int64
	errors           int64
	mu               sync.RWMutex
}

// MessageHandler 消息处理函数类型
type MessageHandler func(message []byte)

// NewKafkaService 创建Kafka服务
func NewKafkaService() (*KafkaService, error) {
	// 创建同步生产者配置
	producerConfig := sarama.NewConfig()
	producerConfig.Producer.RequiredAcks = sarama.WaitForAll
	producerConfig.Producer.Retry.Max = 5
	producerConfig.Producer.Return.Successes = true
	producerConfig.Producer.Compression = sarama.CompressionSnappy   // 添加压缩
	producerConfig.Producer.Flush.Frequency = 500 * time.Millisecond // 批量发送
	producerConfig.Producer.Flush.MaxMessages = 10                   // 最大批量消息数
	producerConfig.Version = sarama.V2_5_0_0                         // 使用更新的Kafka版本

	// 创建同步生产者
	producer, err := sarama.NewSyncProducer(config.AppConfig.KafkaBootstrapServers, producerConfig)
	if err != nil {
		return nil, fmt.Errorf("创建Kafka同步生产者失败: %v", err)
	}

	// 创建异步生产者配置
	asyncConfig := sarama.NewConfig()
	asyncConfig.Producer.RequiredAcks = sarama.WaitForLocal
	asyncConfig.Producer.Compression = sarama.CompressionSnappy
	asyncConfig.Producer.Flush.Frequency = 500 * time.Millisecond
	asyncConfig.Producer.Flush.MaxMessages = 10
	asyncConfig.Producer.Return.Successes = true
	asyncConfig.Producer.Return.Errors = true
	asyncConfig.Version = sarama.V2_5_0_0

	// 创建异步生产者
	asyncProducer, err := sarama.NewAsyncProducer(config.AppConfig.KafkaBootstrapServers, asyncConfig)
	if err != nil {
		producer.Close()
		return nil, fmt.Errorf("创建Kafka异步生产者失败: %v", err)
	}

	// 创建消费者配置
	consumerConfig := sarama.NewConfig()
	consumerConfig.Consumer.Return.Errors = true
	consumerConfig.Consumer.Offsets.Initial = sarama.OffsetNewest // 从最新的偏移量开始消费
	consumerConfig.Consumer.Offsets.AutoCommit.Enable = true
	consumerConfig.Consumer.Offsets.AutoCommit.Interval = 1 * time.Second
	//consumerConfig.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategyRoundRobin // 使用轮询策略
	consumerConfig.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{
		sarama.NewBalanceStrategyRoundRobin(), // 轮询
	}
	consumerConfig.Version = sarama.V2_5_0_0

	// 创建消费者组
	consumer, err := sarama.NewConsumerGroup(config.AppConfig.KafkaBootstrapServers, config.AppConfig.KafkaConsumerGroup, consumerConfig)
	if err != nil {
		producer.Close()
		asyncProducer.Close()
		return nil, fmt.Errorf("创建Kafka消费者组失败: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errorChan := make(chan *sarama.ConsumerError, 100)

	service := &KafkaService{
		producer:      producer,
		asyncProducer: asyncProducer,
		consumer:      consumer,
		topics:        make(map[string]bool),
		handlers:      make(map[string]MessageHandler),
		ctx:           ctx,
		cancel:        cancel,
		errorChan:     errorChan,
		metrics:       &KafkaMetrics{},
	}

	// 处理异步生产者的成功和错误回调
	go service.handleAsyncProducerResponses()

	// 处理消费者错误
	go service.handleConsumerErrors()

	return service, nil
}

// 处理异步生产者的响应
func (s *KafkaService) handleAsyncProducerResponses() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case success := <-s.asyncProducer.Successes():
			if success != nil {
				s.metrics.mu.Lock()
				s.metrics.messagesSent++
				s.metrics.mu.Unlock()
				log.Printf("消息成功发送到主题 %s [分区:%d] @ 偏移量 %d",
					success.Topic, success.Partition, success.Offset)
			}
		case err := <-s.asyncProducer.Errors():
			if err != nil {
				s.metrics.mu.Lock()
				s.metrics.errors++
				s.metrics.mu.Unlock()
				log.Printf("发送消息失败: %v", err)
			}
		}
	}
}

// 处理消费者错误
func (s *KafkaService) handleConsumerErrors() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case err := <-s.errorChan:
			if err != nil {
				s.metrics.mu.Lock()
				s.metrics.errors++
				s.metrics.mu.Unlock()
				log.Printf("消费消息错误: %v", err)
			}
		}
	}
}

// Close 关闭Kafka服务
func (s *KafkaService) Close() error {
	s.cancel()

	var errs []error

	if err := s.producer.Close(); err != nil {
		errs = append(errs, fmt.Errorf("关闭Kafka同步生产者失败: %v", err))
	}

	if err := s.asyncProducer.Close(); err != nil {
		errs = append(errs, fmt.Errorf("关闭Kafka异步生产者失败: %v", err))
	}

	if err := s.consumer.Close(); err != nil {
		errs = append(errs, fmt.Errorf("关闭Kafka消费者失败: %v", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("关闭Kafka服务时发生错误: %v", errs)
	}

	return nil
}

// GetMetrics 获取Kafka指标
func (s *KafkaService) GetMetrics() map[string]int64 {
	s.metrics.mu.RLock()
	defer s.metrics.mu.RUnlock()

	return map[string]int64{
		"messages_sent":     s.metrics.messagesSent,
		"messages_received": s.metrics.messagesReceived,
		"errors":            s.metrics.errors,
	}
}

// EnsureTopicExists 确保主题存在
func (s *KafkaService) EnsureTopicExists(topic string) error {
	s.topicsMutex.RLock()
	exists := s.topics[topic]
	s.topicsMutex.RUnlock()

	if exists {
		return nil
	}

	// 创建管理客户端
	adminConfig := sarama.NewConfig()
	adminConfig.Version = sarama.V2_5_0_0

	admin, err := sarama.NewClusterAdmin(config.AppConfig.KafkaBootstrapServers, adminConfig)
	if err != nil {
		return fmt.Errorf("创建Kafka管理客户端失败: %v", err)
	}
	defer admin.Close()

	// 检查主题是否存在
	topics, err := admin.ListTopics()
	if err != nil {
		return fmt.Errorf("获取主题列表失败: %v", err)
	}

	if _, exists := topics[topic]; !exists {
		// 创建主题
		topicDetail := &sarama.TopicDetail{
			NumPartitions:     int32(config.AppConfig.KafkaPartitions),
			ReplicationFactor: int16(config.AppConfig.KafkaReplicationFactor),
			ConfigEntries: map[string]*string{
				"retention.ms":   strPtr("86400000"), // 1天的消息保留时间
				"cleanup.policy": strPtr("delete"),
			},
		}

		if err := admin.CreateTopic(topic, topicDetail, false); err != nil {
			return fmt.Errorf("创建主题失败: %v", err)
		}

		log.Printf("已创建Kafka主题: %s", topic)
	}

	s.topicsMutex.Lock()
	s.topics[topic] = true
	s.topicsMutex.Unlock()

	return nil
}

// PublishMessage 发布消息到Kafka (同步)
func (s *KafkaService) PublishMessage(topic string, key string, message []byte) error {
	// 确保主题存在
	if err := s.EnsureTopicExists(topic); err != nil {
		return err
	}

	// 创建消息
	msg := &sarama.ProducerMessage{
		Topic:     topic,
		Value:     sarama.ByteEncoder(message),
		Timestamp: time.Now(),
	}

	if key != "" {
		msg.Key = sarama.StringEncoder(key)
	}

	// 发送消息
	partition, offset, err := s.producer.SendMessage(msg)
	if err != nil {
		s.metrics.mu.Lock()
		s.metrics.errors++
		s.metrics.mu.Unlock()
		return fmt.Errorf("发送消息失败: %v", err)
	}

	s.metrics.mu.Lock()
	s.metrics.messagesSent++
	s.metrics.mu.Unlock()

	log.Printf("消息已发送到主题 %s [分区:%d] @ 偏移量 %d", topic, partition, offset)
	return nil
}

// PublishMessageAsync 异步发布消息到Kafka
func (s *KafkaService) PublishMessageAsync(topic string, key string, message []byte) {
	// 确保主题存在 (异步方式)
	go func() {
		if err := s.EnsureTopicExists(topic); err != nil {
			log.Printf("确保主题存在失败: %v", err)
			return
		}

		// 创建消息
		msg := &sarama.ProducerMessage{
			Topic:     topic,
			Value:     sarama.ByteEncoder(message),
			Timestamp: time.Now(),
		}

		if key != "" {
			msg.Key = sarama.StringEncoder(key)
		}

		// 异步发送消息
		s.asyncProducer.Input() <- msg
	}()
}

// SubscribeTopic 订阅主题
func (s *KafkaService) SubscribeTopic(topic string, handler MessageHandler) error {
	// 确保主题存在
	if err := s.EnsureTopicExists(topic); err != nil {
		return err
	}

	// 注册处理函数
	s.handlerMutex.Lock()
	s.handlers[topic] = handler
	s.handlerMutex.Unlock()

	// 启动消费者
	go func() {
		// 创建消费者处理器
		handler := &kafkaConsumerHandler{
			ready:   make(chan bool),
			service: s,
			topic:   topic,
		}

		for {
			select {
			case <-s.ctx.Done():
				return
			default:
				// 消费消息
				if err := s.consumer.Consume(s.ctx, []string{topic}, handler); err != nil {
					if err == sarama.ErrClosedConsumerGroup {
						return
					}
					log.Printf("消费主题 %s 失败: %v", topic, err)
					time.Sleep(5 * time.Second) // 重试前等待
					continue
				}

				// 检查上下文是否已取消
				if s.ctx.Err() != nil {
					return
				}

				// 等待消费者就绪
				<-handler.ready
			}
		}
	}()

	log.Printf("已订阅主题: %s", topic)
	return nil
}

// kafkaConsumerHandler 实现sarama.ConsumerGroupHandler接口
type kafkaConsumerHandler struct {
	ready   chan bool
	service *KafkaService
	topic   string
}

// Setup 在消费者会话开始时调用
func (h *kafkaConsumerHandler) Setup(sarama.ConsumerGroupSession) error {
	close(h.ready)
	return nil
}

// Cleanup 在消费者会话结束时调用
func (h *kafkaConsumerHandler) Cleanup(sarama.ConsumerGroupSession) error {
	h.ready = make(chan bool)
	return nil
}

// ConsumeClaim 消费消息
func (h *kafkaConsumerHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case message, ok := <-claim.Messages():
			if !ok {
				return nil
			}

			// 处理消息
			h.service.handlerMutex.RLock()
			handler := h.service.handlers[h.topic]
			h.service.handlerMutex.RUnlock()

			if handler != nil {
				// 使用goroutine处理消息，避免阻塞消费者
				go func(msg *sarama.ConsumerMessage) {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("处理消息时发生panic: %v", r)
						}
					}()

					handler(msg.Value)

					h.service.metrics.mu.Lock()
					h.service.metrics.messagesReceived++
					h.service.metrics.mu.Unlock()
				}(message)
			}

			// 标记消息为已处理
			session.MarkMessage(message, "")

		case <-session.Context().Done():
			return nil
		}
	}
}

// BuildTopicName 构建主题名称
func (s *KafkaService) BuildTopicName(topicType string, id uint) string {
	return fmt.Sprintf("%s%s-%d", config.AppConfig.KafkaTopicPrefix, topicType, id)
}

// PublishChatMessage 发布聊天消息
func (s *KafkaService) PublishChatMessage(msgType string, message []byte, receiverID, groupID uint) error {
	var topic string
	var key string

	if groupID > 0 {
		// 群组消息
		topic = s.BuildTopicName("group", groupID)
		key = fmt.Sprintf("group-%d", groupID)
	} else if receiverID > 0 {
		// 私聊消息
		topic = s.BuildTopicName("private", receiverID)
		key = fmt.Sprintf("user-%d", receiverID)
	} else {
		// 全局消息
		topic = s.BuildTopicName("global", 0)
		key = "global"
	}

	// 包装消息
	wrapper := struct {
		Type      string          `json:"type"`
		Content   json.RawMessage `json:"content"`
		Timestamp time.Time       `json:"timestamp"`
	}{
		Type:      msgType,
		Content:   message,
		Timestamp: time.Now(),
	}

	wrapperJSON, err := json.Marshal(wrapper)
	if err != nil {
		return fmt.Errorf("序列化消息失败: %v", err)
	}

	// 根据消息类型选择同步或异步发送
	if msgType == "chat_message" || msgType == "system" {
		// 重要消息使用同步发送确保可靠性
		return s.PublishMessage(topic, key, wrapperJSON)
	} else {
		// 非关键消息使用异步发送提高性能
		s.PublishMessageAsync(topic, key, wrapperJSON)
		return nil
	}
}

// CreateConsumerGroup 创建新的消费者组
func (s *KafkaService) CreateConsumerGroup(groupID string) (sarama.ConsumerGroup, error) {
	consumerConfig := sarama.NewConfig()
	consumerConfig.Consumer.Return.Errors = true
	consumerConfig.Consumer.Offsets.Initial = sarama.OffsetNewest
	//consumerConfig.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategyRoundRobin
	consumerConfig.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{
		sarama.NewBalanceStrategyRoundRobin(), // 轮询
	}
	consumerConfig.Version = sarama.V2_5_0_0

	return sarama.NewConsumerGroup(config.AppConfig.KafkaBootstrapServers, groupID, consumerConfig)
}

func strPtr(s string) *string {
	return &s
}
