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
type FireTower struct {
	connID    uint64 // 连接id 每台服务器上该id从1开始自增
	clientID  string // 客户端id 用来做业务逻辑
	userID    string // 一般业务中每个连接都是一个用户 用来给业务提供用户识别
	ext       *sync.Map
	Cookie    []byte // 这里提供给业务放一个存放跟当前连接相关的数据信息
	startTime time.Time

	logger    *zap.Logger
	timeout   time.Duration
	readIn    chan *protocol.FireInfo // 读取队列
	sendOut   chan []byte             // 发送队列
	ws        *websocket.Conn         // 保存底层websocket连接
	topic     sync.Map                // 订阅topic列表
	isClose   bool                    // 判断当前websocket是否被关闭
	closeChan chan struct{}           // 用来作为关闭websocket的触发点
	mutex     sync.Mutex              // 避免并发close chan

	onConnectHandler       func() bool
	onOfflineHandler       func()
	receivedHandler        func(protocol.ReadOnlyFire) bool
	readHandler            func(protocol.ReadOnlyFire) bool
	readTimeoutHandler     func(protocol.ReadOnlyFire)
	sendTimeoutHandler     func(protocol.ReadOnlyFire)
	subscribeHandler       func(context protocol.FireLife, topic []string)
	unSubscribeHandler     func(context protocol.FireLife, topic []string)
	beforeSubscribeHandler func(context protocol.FireLife, topic []string) bool
	onSystemRemove         func(topic string)
}

func (t *FireTower) ClientID() string {
	return t.clientID
}

func (t *FireTower) UserID() string {
	return t.userID
}

func (t *FireTower) SetUserID(id string) {
	t.userID = id
}

func (t *FireTower) Ext() *sync.Map {
	return t.ext
}

// Init 初始化firetower
// 在调用firetower前请一定要先调用Init方法
func Setup(cfg config.FireTowerConfig, opts ...TowerOption) (Manager, error) {
	towerPool.New = func() interface{} {
		return &FireTower{}
	}
	// 构建服务架构
	return BuildFoundation(cfg, opts...)
}

// BuildTower 实例化一个websocket客户端
func BuildTower(ws *websocket.Conn, clientId string) (tower *FireTower, err error) {
	if ws == nil {
		tm.logger.Error("empty websocket connect")
		return nil, errors.New("empty websocket connect")
	}
	tower = buildNewTower(ws, clientId)
	return
}

func buildNewTower(ws *websocket.Conn, clientID string) *FireTower {
	t := towerPool.Get().(*FireTower)
	t.connID = getConnId()
	t.clientID = clientID
	t.startTime = time.Now()
	t.ext = &sync.Map{}
	t.readIn = make(chan *protocol.FireInfo, tm.cfg.ReadChanLens)
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

func (t *FireTower) OnClose() chan struct{} {
	return t.closeChan
}

// Run 启动websocket客户端
func (t *FireTower) Run() {
	t.logger.Debug("new tower builded")
	tm.connCounter <- counterMsg{
		Key: tm.ip,
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

func (t *FireTower) TopicList() []string {
	var topics []string
	t.topic.Range(func(key, value any) bool {
		topics = append(topics, key.(string))
		return true
	})
	return topics
}

// 订阅topic的绑定过程
func (t *FireTower) bindTopic(topic []string) ([]string, error) {
	var (
		addTopic []string
	)
	bucket := tm.GetBucket(t)
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

func (t *FireTower) unbindTopic(topic []string) []string {
	var delTopic []string // 待取消订阅的topic列表
	bucket := tm.GetBucket(t)
	for _, v := range topic {
		if _, loaded := t.topic.LoadAndDelete(v); loaded {
			// 如果客户端已经订阅过该topic才执行退订
			delTopic = append(delTopic, v)
			bucket.DelSubscribe(v, t)
		}
	}
	return delTopic
}

func (t *FireTower) read() (*protocol.FireInfo, error) {
	if t.isClose {
		return nil, ErrorClose
	}
	return <-t.readIn, nil
}

// Close 关闭客户端连接并注销
// 调用该方法会完全注销掉由BuildTower生成的一切内容
func (t *FireTower) Close() {
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

			fire := NewFire(protocol.SourceSystem, t)
			defer tm.brazier.Extinguished(fire)

			if t.unSubscribeHandler != nil {
				t.unSubscribeHandler(fire.Context, delTopic)
			}
		}

		if t.ws != nil {
			t.ws.Close()
		}

		tm.connCounter <- counterMsg{
			Key: tm.ip,
			Num: -1,
		}
		close(t.closeChan)
		if t.onOfflineHandler != nil {
			t.onOfflineHandler()
		}
		towerPool.Put(t)
	}
}

var heartbeat = []byte{104, 101, 97, 114, 116, 98, 101, 97, 116}

func (t *FireTower) sendLoop() {
	heartTicker := time.NewTicker(time.Duration(tm.cfg.Heartbeat) * time.Second)
	for {
		select {
		case wsMsg := <-t.sendOut:
			if t.ws != nil {
				if err := t.SendToClient(wsMsg); err != nil {
					goto collapse
				}
			}
		case <-heartTicker.C:
			// sendMessage.Data = []byte{104, 101, 97, 114, 116, 98, 101, 97, 116} // []byte("heartbeat")
			if err := t.SendToClient(heartbeat); err != nil {
				goto collapse
			}
		case <-t.closeChan:
			heartTicker.Stop()
			return
		}
	}
collapse:
	t.Close()
}

func (t *FireTower) readLoop() {
	if t.ws == nil {
		return
	}
	defer func() {
		if err := recover(); err != nil {
			tm.logger.Error("readloop panic", zap.Any("error", err))
		}
	}()
	for {
		messageType, data, err := t.ws.ReadMessage()
		if err != nil { // 断开连接
			goto collapse // 出现问题烽火台直接坍塌
		}
		fire := NewFire(protocol.SourceClient, t) // 从对象池中获取消息对象 降低GC压力
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
			tm.brazier.Extinguished(fire)
		}()

	}
collapse:
	t.Close()
}

// 处理前端发来的数据
// 这里要做逻辑拆分，判断用户是要进行通信还是topic订阅
func (t *FireTower) readDispose() {
	for {
		fire, err := t.read()
		if err != nil {
			t.logger.Error("failed to read message from websocket, tower will be closed", zap.Error(err))
			t.Close()
			return
		}
		go t.readLogic(fire)
	}
}

func (t *FireTower) readLogic(fire *protocol.FireInfo) error {
	defer tm.brazier.Extinguished(fire)
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

func (t *FireTower) Subscribe(context protocol.FireLife, topics []string) error {
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

func (t *FireTower) UnSubscribe(context protocol.FireLife, topics []string) error {
	delTopics := t.unbindTopic(topics)

	if t.unSubscribeHandler != nil {
		t.unSubscribeHandler(context, delTopics)
	}
	return nil
}

// Publish 推送接口
// 通过BuildTower生成的实例都可以调用该方法来达到推送的目的
func (t *FireTower) Publish(fire *protocol.FireInfo) error {
	err := tm.Publish(fire)
	if err != nil {
		t.logger.Error("failed to publish message", zap.Error(err))
		return err
	}
	return nil
}

// SendToClient 向自己推送消息
// 这里描述一下使用场景
// 只针对当前客户端进行的推送请调用该方法
func (t *FireTower) SendToClient(b []byte) error {
	if t.ws == nil {
		return ErrorServerSideMode
	}
	t.mutex.Lock()
	defer t.mutex.Unlock()
	if t.isClose != true {
		return t.ws.WriteMessage(websocket.TextMessage, b)
	}
	return ErrorClose
}

// SetOnConnectHandler 建立连接事件
func (t *FireTower) SetOnConnectHandler(fn func() bool) {
	t.onConnectHandler = fn
}

// SetOnOfflineHandler 用户连接关闭时触发
func (t *FireTower) SetOnOfflineHandler(fn func()) {
	t.onOfflineHandler = fn
}

func (t *FireTower) SetReceivedHandler(fn func(protocol.ReadOnlyFire) bool) {
	t.receivedHandler = fn
}

// SetReadHandler 客户端推送事件
// 接收到用户publish的消息时触发
func (t *FireTower) SetReadHandler(fn func(protocol.ReadOnlyFire) bool) {
	t.readHandler = fn
}

// SetSubscribeHandler 订阅事件
// 用户订阅topic后触发
func (t *FireTower) SetSubscribeHandler(fn func(context protocol.FireLife, topic []string)) {
	t.subscribeHandler = fn
}

// SetUnSubscribeHandler 取消订阅事件
// 用户取消订阅topic后触发
func (t *FireTower) SetUnSubscribeHandler(fn func(context protocol.FireLife, topic []string)) {
	t.unSubscribeHandler = fn
}

// SetBeforeSubscribeHandler 订阅前回调事件
// 用户订阅topic前触发
func (t *FireTower) SetBeforeSubscribeHandler(fn func(context protocol.FireLife, topic []string) bool) {
	t.beforeSubscribeHandler = fn
}

// SetReadTimeoutHandler 超时回调
// readIn channal写满了  生产 > 消费的情况下触发超时机制
func (t *FireTower) SetReadTimeoutHandler(fn func(protocol.ReadOnlyFire)) {
	t.readTimeoutHandler = fn
}

func (t *FireTower) SetSendTimeoutHandler(fn func(protocol.ReadOnlyFire)) {
	t.sendTimeoutHandler = fn
}

// SetOnSystemRemove 系统移除某个用户的topic订阅
func (t *FireTower) SetOnSystemRemove(fn func(topic string)) {
	t.onSystemRemove = fn
}

// GetConnectNum 获取话题订阅数的grpc方法封装
func (t *FireTower) GetConnectNum(topic string) (uint64, error) {
	number, err := tm.stores.ClusterTopicStore().GetTopicConnNum(topic)
	if err != nil {
		return 0, err
	}
	return number, nil
}

func (t *FireTower) Logger() *zap.Logger {
	return t.logger
}
