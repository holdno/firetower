package tower

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/holdno/firetower/config"
	"github.com/holdno/firetower/protocol"
	json "github.com/json-iterator/go"
)

var (
	towerPool sync.Pool
)

// FireTower 客户端连接结构体
// 包含了客户端一个连接的所有信息
type FireTower[T any] struct {
	tm        *TowerManager[T]
	connID    uint64 // 连接id 每台服务器上该id从1开始自增
	clientID  string // 客户端id 用来做业务逻辑
	userID    string // 一般业务中每个连接都是一个用户 用来给业务提供用户识别
	ext       *sync.Map
	Cookie    []byte // 这里提供给业务放一个存放跟当前连接相关的数据信息
	startTime time.Time

	logger    *zap.Logger
	timeout   time.Duration
	readIn    chan *protocol.FireInfo[T] // 读取队列
	sendOut   chan []byte                // 发送队列
	ws        *websocket.Conn            // 保存底层websocket连接
	topic     sync.Map                   // 订阅topic列表
	isClose   bool                       // 判断当前websocket是否被关闭
	closeChan chan struct{}              // 用来作为关闭websocket的触发点
	mutex     sync.Mutex                 // 避免并发close chan

	onConnectHandler       func() bool
	onOfflineHandler       func()
	receivedHandler        func(protocol.ReadOnlyFire[T]) bool
	readHandler            func(protocol.ReadOnlyFire[T]) bool
	readTimeoutHandler     func(protocol.ReadOnlyFire[T])
	sendTimeoutHandler     func(protocol.ReadOnlyFire[T])
	subscribeHandler       func(context protocol.FireLife, topic []string)
	unSubscribeHandler     func(context protocol.FireLife, topic []string)
	beforeSubscribeHandler func(context protocol.FireLife, topic []string) bool
	onSystemRemove         func(topic string)
}

func (t *FireTower[T]) ClientID() string {
	return t.clientID
}

func (t *FireTower[T]) UserID() string {
	return t.userID
}

func (t *FireTower[T]) SetUserID(id string) {
	t.userID = id
}

func (t *FireTower[T]) Ext() *sync.Map {
	return t.ext
}

// Init 初始化firetower
// 在调用firetower前请一定要先调用Init方法
func Setup[T any](cfg config.FireTowerConfig, opts ...TowerOption[T]) (Manager[T], error) {
	towerPool.New = func() interface{} {
		return &FireTower[T]{}
	}
	// 构建服务架构
	return BuildFoundation(cfg, opts...)
}

// BuildTower 实例化一个websocket客户端
func (t *TowerManager[T]) BuildTower(ws *websocket.Conn, clientId string) (tower *FireTower[T], err error) {
	if ws == nil {
		t.logger.Error("empty websocket connect")
		return nil, errors.New("empty websocket connect")
	}
	tower = buildNewTower(t, ws, clientId)
	return
}

func buildNewTower[T any](tm *TowerManager[T], ws *websocket.Conn, clientID string) *FireTower[T] {
	t := towerPool.Get().(*FireTower[T])
	t.tm = tm
	t.connID = getConnId()
	t.clientID = clientID
	t.startTime = time.Now()
	t.ext = &sync.Map{}
	t.readIn = make(chan *protocol.FireInfo[T], tm.cfg.ReadChanLens)
	t.sendOut = make(chan []byte, tm.cfg.WriteChanLens)
	t.topic = sync.Map{}
	t.ws = ws
	t.isClose = false
	t.closeChan = make(chan struct{})
	t.timeout = time.Second * 3

	t.readHandler = nil
	t.readTimeoutHandler = nil
	t.sendTimeoutHandler = nil
	t.subscribeHandler = nil
	t.unSubscribeHandler = nil
	t.beforeSubscribeHandler = nil

	t.logger = tm.logger.With(zap.String("client_id", t.clientID), zap.String("user_id", t.userID))

	return t
}

func (t *FireTower[T]) OnClose() chan struct{} {
	return t.closeChan
}

// Run 启动websocket客户端
func (t *FireTower[T]) Run() {
	t.logger.Debug("new tower builded")
	t.tm.connCounter <- counterMsg{
		Key: t.tm.ip,
		Num: 1,
	}

	if t.ws == nil {
		// server side
		return
	}

	// 读取websocket信息
	go t.readLoop()
	// 处理读取事件
	go t.readDispose()

	if t.onConnectHandler != nil {
		ok := t.onConnectHandler()
		if !ok {
			t.Close()
		}
	}
	// 向websocket发送信息
	t.sendLoop()
}

func (t *FireTower[T]) TopicList() []string {
	var topics []string
	t.topic.Range(func(key, value any) bool {
		topics = append(topics, key.(string))
		return true
	})
	return topics
}

// 订阅topic的绑定过程
func (t *FireTower[T]) bindTopic(topic []string) ([]string, error) {
	var (
		addTopic []string
	)
	bucket := t.tm.GetBucket(t)
	for _, v := range topic {
		if _, loaded := t.topic.LoadOrStore(v, struct{}{}); !loaded {
			addTopic = append(addTopic, v) // 待订阅的topic
			bucket.AddSubscribe(v, t)
		}
		// if _, ok := t.topic[v]; !ok {
		// 	addTopic = append(addTopic, v) // 待订阅的topic
		// 	t.topic[v] = true
		// 	bucket.AddSubscribe(v, t)
		// }
	}
	return addTopic, nil
}

func (t *FireTower[T]) unbindTopic(topic []string) []string {
	var delTopic []string // 待取消订阅的topic列表
	bucket := t.tm.GetBucket(t)
	for _, v := range topic {
		if _, loaded := t.topic.LoadAndDelete(v); loaded {
			// 如果客户端已经订阅过该topic才执行退订
			delTopic = append(delTopic, v)
			bucket.DelSubscribe(v, t)
		}
	}
	return delTopic
}

func (t *FireTower[T]) read() (*protocol.FireInfo[T], error) {
	if t.isClose {
		return nil, ErrorClosed
	}
	fire, ok := <-t.readIn
	if !ok {
		return nil, ErrorClosed
	}
	return fire, nil
}

// Close 关闭客户端连接并注销
// 调用该方法会完全注销掉由BuildTower生成的一切内容
func (t *FireTower[T]) Close() {
	t.logger.Debug("close connect")
	t.mutex.Lock()
	defer t.mutex.Unlock()
	if !t.isClose {
		t.isClose = true
		var topicSlice []string
		t.topic.Range(func(key, value any) bool {
			topicSlice = append(topicSlice, key.(string))
			return true
		})
		if len(topicSlice) > 0 {
			delTopic := t.unbindTopic(topicSlice)

			fire := t.tm.NewFire(protocol.SourceSystem, t)
			defer t.tm.brazier.Extinguished(fire)

			if t.unSubscribeHandler != nil {
				t.unSubscribeHandler(fire.Context, delTopic)
			}
		}

		if t.ws != nil {
			t.ws.Close()
		}

		t.tm.connCounter <- counterMsg{
			Key: t.tm.ip,
			Num: -1,
		}
		close(t.closeChan)
		if t.onOfflineHandler != nil {
			t.onOfflineHandler()
		}
		towerPool.Put(t)
		t.logger.Debug("tower closed")
	}
}

var heartbeat = []byte{104, 101, 97, 114, 116, 98, 101, 97, 116}

func (t *FireTower[T]) sendLoop() {
	heartTicker := time.NewTicker(time.Duration(t.tm.cfg.Heartbeat) * time.Second)
	defer func() {
		heartTicker.Stop()
		t.Close()
		close(t.sendOut)
	}()
	for {
		select {
		case wsMsg := <-t.sendOut:
			if t.ws != nil {
				if err := t.SendToClient(wsMsg); err != nil {
					return
				}
			}
		case <-heartTicker.C:
			// sendMessage.Data = []byte{104, 101, 97, 114, 116, 98, 101, 97, 116} // []byte("heartbeat")
			if err := t.SendToClient(heartbeat); err != nil {
				return
			}
		case <-t.closeChan:
			return
		}
	}
}

func (t *FireTower[T]) readLoop() {
	if t.ws == nil {
		return
	}
	defer func() {
		if err := recover(); err != nil {
			t.tm.logger.Error("readloop panic", zap.Any("error", err))
		}
		t.Close()
		close(t.readIn)
	}()
	for {
		messageType, data, err := t.ws.ReadMessage()
		if err != nil { // 断开连接
			return
		}
		fire := t.tm.NewFire(protocol.SourceClient, t) // 从对象池中获取消息对象 降低GC压力
		fire.MessageType = messageType
		if err := json.Unmarshal(data, &fire.Message); err != nil {
			t.logger.Error("failed to unmarshal client data, filtered", zap.Error(err))
			continue
		}

		func() {
			ctx, cancel := context.WithTimeout(context.Background(), t.timeout)
			defer cancel()
			select {
			case t.readIn <- fire:
				return
			case <-ctx.Done():
				if t.readTimeoutHandler != nil {
					t.readTimeoutHandler(fire)
				}
				t.logger.Error("readloop timeout", zap.Any("data", data))
			case <-t.closeChan:
			}
			t.tm.brazier.Extinguished(fire)
		}()

	}
}

// 处理前端发来的数据
// 这里要做逻辑拆分，判断用户是要进行通信还是topic订阅
func (t *FireTower[T]) readDispose() {
	for {
		fire, err := t.read()
		if err != nil {
			t.logger.Error("failed to read message from websocket", zap.Error(err))
			return
		}
		if fire != nil {
			go t.readLogic(fire)
		}
	}
}

func (t *FireTower[T]) readLogic(fire *protocol.FireInfo[T]) error {
	defer t.tm.brazier.Extinguished(fire)
	if t.isClose {
		return nil
	} else {
		if fire.Message.Topic == "" {
			t.logger.Error("the obtained topic is empty. this message will be filtered")
			return fmt.Errorf("%s:topic is empty, ClintId:%s, UserId:%s", fire.Message.Type, t.ClientID(), t.UserID())
		}
		switch fire.Message.Type {
		case protocol.SubscribeOperation: // 客户端订阅topic
			addTopics := strings.Split(fire.Message.Topic, ",")
			// 增加messageId 方便追踪
			err := t.Subscribe(fire.Context, addTopics)
			if err != nil {
				t.logger.Error("failed to subscribe topics", zap.Strings("topics", addTopics), zap.Error(err))
				// TODO metrics
				return err
			}
		case protocol.UnSubscribeOperation: // 客户端取消订阅topic
			delTopic := strings.Split(fire.Message.Topic, ",")
			err := t.UnSubscribe(fire.Context, delTopic)
			if err != nil {
				t.logger.Error("failed to unsubscribe topics", zap.Strings("topics", delTopic), zap.Error(err))
				// TODO metrics
				return err
			}
		default:
			if t.readHandler != nil && !t.readHandler(fire) {
				return nil
			}
			t.Publish(fire)
		}
	}
	return nil
}

func (t *FireTower[T]) Subscribe(context protocol.FireLife, topics []string) error {
	// 如果设置了订阅前触发事件则调用
	if t.beforeSubscribeHandler != nil {
		ok := t.beforeSubscribeHandler(context, topics)
		if !ok {
			return nil
		}
	}
	addTopics, err := t.bindTopic(topics)
	if err != nil {
		return err
	}
	if t.subscribeHandler != nil {
		t.subscribeHandler(context, addTopics)
	}
	return nil
}

func (t *FireTower[T]) UnSubscribe(context protocol.FireLife, topics []string) error {
	delTopics := t.unbindTopic(topics)

	if t.unSubscribeHandler != nil {
		t.unSubscribeHandler(context, delTopics)
	}
	return nil
}

// Publish 推送接口
// 通过BuildTower生成的实例都可以调用该方法来达到推送的目的
func (t *FireTower[T]) Publish(fire *protocol.FireInfo[T]) error {
	err := t.tm.Publish(fire)
	if err != nil {
		t.logger.Error("failed to publish message", zap.Error(err))
		return err
	}
	return nil
}

// SendToClient 向自己推送消息
// 这里描述一下使用场景
// 只针对当前客户端进行的推送请调用该方法
func (t *FireTower[T]) SendToClient(b []byte) error {
	if t.ws == nil {
		return ErrorServerSideMode
	}

	if t.isClose != true {
		return t.ws.WriteMessage(websocket.TextMessage, b)
	}
	return ErrorClosed
}

// SetOnConnectHandler 建立连接事件
func (t *FireTower[T]) SetOnConnectHandler(fn func() bool) {
	t.onConnectHandler = fn
}

// SetOnOfflineHandler 用户连接关闭时触发
func (t *FireTower[T]) SetOnOfflineHandler(fn func()) {
	t.onOfflineHandler = fn
}

func (t *FireTower[T]) SetReceivedHandler(fn func(protocol.ReadOnlyFire[T]) bool) {
	t.receivedHandler = fn
}

// SetReadHandler 客户端推送事件
// 接收到用户publish的消息时触发
func (t *FireTower[T]) SetReadHandler(fn func(protocol.ReadOnlyFire[T]) bool) {
	t.readHandler = fn
}

// SetSubscribeHandler 订阅事件
// 用户订阅topic后触发
func (t *FireTower[T]) SetSubscribeHandler(fn func(context protocol.FireLife, topic []string)) {
	t.subscribeHandler = fn
}

// SetUnSubscribeHandler 取消订阅事件
// 用户取消订阅topic后触发
func (t *FireTower[T]) SetUnSubscribeHandler(fn func(context protocol.FireLife, topic []string)) {
	t.unSubscribeHandler = fn
}

// SetBeforeSubscribeHandler 订阅前回调事件
// 用户订阅topic前触发
func (t *FireTower[T]) SetBeforeSubscribeHandler(fn func(context protocol.FireLife, topic []string) bool) {
	t.beforeSubscribeHandler = fn
}

// SetReadTimeoutHandler 超时回调
// readIn channal写满了  生产 > 消费的情况下触发超时机制
func (t *FireTower[T]) SetReadTimeoutHandler(fn func(protocol.ReadOnlyFire[T])) {
	t.readTimeoutHandler = fn
}

func (t *FireTower[T]) SetSendTimeoutHandler(fn func(protocol.ReadOnlyFire[T])) {
	t.sendTimeoutHandler = fn
}

// SetOnSystemRemove 系统移除某个用户的topic订阅
func (t *FireTower[T]) SetOnSystemRemove(fn func(topic string)) {
	t.onSystemRemove = fn
}

// GetConnectNum 获取话题订阅数的grpc方法封装
func (t *FireTower[T]) GetConnectNum(topic string) (uint64, error) {
	number, err := t.tm.stores.ClusterTopicStore().GetTopicConnNum(topic)
	if err != nil {
		return 0, err
	}
	return number, nil
}

func (t *FireTower[T]) Logger() *zap.Logger {
	return t.logger
}
