package tower

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/holdno/firetower/config"
	"github.com/holdno/firetower/protocol"
	json "github.com/json-iterator/go"
)

var (
	// TowerLogger 接管系统log t log类型 info log信息
	TowerLogger func(t *FireTower, types, info string)

	towerPool sync.Pool
)

// FireTower 客户端连接结构体
// 包含了客户端一个连接的所有信息
type FireTower struct {
	connID    uint64 // 连接id 每台服务器上该id从1开始自增
	clientID  string // 客户端id 用来做业务逻辑
	userID    string // 一般业务中每个连接都是一个用户 用来给业务提供用户识别
	ext       sync.Map
	Cookie    []byte // 这里提供给业务放一个存放跟当前连接相关的数据信息
	startTime time.Time

	readIn    chan *protocol.FireInfo // 读取队列
	sendOut   chan *protocol.FireInfo // 发送队列
	ws        *websocket.Conn         // 保存底层websocket连接
	topic     map[string]bool         // 订阅topic列表
	isClose   bool                    // 判断当前websocket是否被关闭
	closeChan chan struct{}           // 用来作为关闭websocket的触发点
	mutex     sync.Mutex              // 避免并发close chan

	onConnectHandler       func() bool
	onOfflineHandler       func()
	receivedHandler        func(*protocol.FireInfo) bool
	readHandler            func(*protocol.FireInfo) bool
	readTimeoutHandler     func(*protocol.FireInfo)
	subscribeHandler       func(context protocol.FireLife, topic []string) bool
	unSubscribeHandler     func(context protocol.FireLife, topic []string) bool
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

func (t *FireTower) Ext() sync.Map {
	return t.ext
}

// Init 初始化firetower
// 在调用firetower前请一定要先调用Init方法
func Setup(cfg config.FireTowerConfig) error {
	towerPool.New = func() interface{} {
		return &FireTower{}
	}

	TowerLogger = towerLog

	// 构建服务架构
	if err := BuildFoundation(cfg); err != nil {
		return err
	}

	return nil
}

// BuildTower 实例化一个websocket客户端
func BuildTower(ws *websocket.Conn, clientId string) (tower *FireTower) {
	if ws == nil {
		towerLog(tower, "ERROR", "websocket.Conn is nil")
		return
	}
	tower = buildNewTower(ws, clientId)
	return
}

func buildNewTower(ws *websocket.Conn, clientID string) *FireTower {
	t := towerPool.Get().(*FireTower)
	t.connID = getConnId()
	t.clientID = clientID
	t.startTime = time.Now()
	t.readIn = make(chan *protocol.FireInfo, tm.cfg.ChanLens)
	t.sendOut = make(chan *protocol.FireInfo, tm.cfg.ChanLens)
	t.topic = make(map[string]bool)
	t.ws = ws
	t.isClose = false
	t.closeChan = make(chan struct{})

	t.readHandler = nil
	t.readTimeoutHandler = nil
	t.subscribeHandler = nil
	t.unSubscribeHandler = nil
	t.beforeSubscribeHandler = nil
	return t
}

func (t *FireTower) OnClose() chan struct{} {
	return t.closeChan
}

// Run 启动websocket客户端
func (t *FireTower) Run() {
	logInfo(t, "new websocket running")
	tm.connCounter <- counterMsg{
		Key: tm.ip,
		Num: 1,
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
	for k := range t.topic {
		topics = append(topics, k)
	}
	return topics
}

// 订阅topic的绑定过程
func (t *FireTower) bindTopic(topic []string) ([]string, error) {
	var (
		addTopic []string
	)
	bucket := tm.GetBucket(t)
	for _, v := range topic {
		if _, ok := t.topic[v]; !ok {
			addTopic = append(addTopic, v) // 待订阅的topic
			t.topic[v] = true
			bucket.AddSubscribe(v, t)
		}
	}
	return addTopic, nil
}

func (t *FireTower) unbindTopic(topic []string) ([]string, error) {
	var delTopic []string // 待取消订阅的topic列表
	bucket := tm.GetBucket(t)
	for _, v := range topic {
		if _, ok := t.topic[v]; ok {
			// 如果客户端已经订阅过该topic才执行退订
			delTopic = append(delTopic, v)
			delete(t.topic, v)
			bucket.DelSubscribe(v, t)
		}
	}
	return delTopic, nil
}

func (t *FireTower) read() (*protocol.FireInfo, error) {
	if t.isClose {
		return nil, ErrorClose
	}
	return <-t.readIn, nil
}

// Send 发送消息方法
// 向某个topic发送某段信息
func (t *FireTower) Send(fire *protocol.FireInfo) error {
	if t.receivedHandler != nil && !t.receivedHandler(fire) {
		return nil
	}
	if t.isClose {
		return ErrorClose
	}
	t.sendOut <- fire
	return nil
}

// Close 关闭客户端连接并注销
// 调用该方法会完全注销掉由BuildTower生成的一切内容
func (t *FireTower) Close() {
	logInfo(t, "websocket connect is closed")
	t.mutex.Lock()
	defer t.mutex.Unlock()
	if !t.isClose {
		t.isClose = true
		if t.topic != nil {
			var topicSlice []string
			for k := range t.topic {
				topicSlice = append(topicSlice, k)
			}
			delTopic, err := t.unbindTopic(topicSlice)
			fire := NewFire(protocol.SourceSystem, t)
			defer tm.brazier.Extinguished(fire)
			if err != nil {
				fire.Panic(err.Error())
			} else {
				if t.unSubscribeHandler != nil {
					t.unSubscribeHandler(fire.Context, delTopic)
				}
			}
		}
		t.ws.Close()
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

func (t *FireTower) sendLoop() {
	heartTicker := time.NewTicker(time.Duration(tm.cfg.Heartbeat) * time.Second)
	for {
		select {
		case fire := <-t.sendOut:
			if fire.MessageType == 0 {
				fire.MessageType = 1 // 文本格式
			}
			if err := t.ws.WriteMessage(fire.MessageType, []byte(fire.Message.Data)); err != nil {
				fire.Panic(err.Error())
				goto collapse
			}
		case <-heartTicker.C:
			// sendMessage.Data = []byte{104, 101, 97, 114, 116, 98, 101, 97, 116} // []byte("heartbeat")
			if err := t.ToSelf([]byte{104, 101, 97, 114, 116, 98, 101, 97, 116}); err != nil {
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
	defer func() {
		if err := recover(); err != nil {
			towerLog(t, "PANIC", fmt.Sprintf("%s", err))
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
			fire.Panic(fmt.Sprintf("client sended data was unmarshal error:%v", err))
			continue
		}

		timeout := time.After(time.Duration(3) * time.Second)
		select {
		case t.readIn <- fire:
		case <-timeout:
			if t.readTimeoutHandler != nil {
				t.readTimeoutHandler(fire)
			}
			fire.Panic(fmt.Sprintf("readLoop timeout: %q", data))
		case <-t.closeChan:
			return
		}
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
			fire.Panic(fmt.Sprintf("read message failed:%v", err))
			t.Close()
			return
		}
		// protocol.RecycleIn(fire, "readDispose")
		defer tm.brazier.Extinguished(fire)
		if t.isClose {
			return
		} else {
			if fire.Message.Topic == "" {
				fire.Panic(fmt.Sprintf("%s:topic is empty, ClintId:%s, UserId:%s", fire.Message.Type, t.ClientID(), t.UserID()))
				continue
			}
			switch fire.Message.Type {
			case "subscribe": // 客户端订阅topic
				addTopics := strings.Split(fire.Message.Topic, ",")
				// 增加messageId 方便追踪
				err = t.Subscribe(fire.Context, addTopics)
				if err != nil {
					fire.Error(err.Error())
				}
			case "unSubscribe": // 客户端取消订阅topic
				delTopic := strings.Split(fire.Message.Topic, ",")
				err = t.UnSubscribe(fire.Context, delTopic)
				if err != nil {
					fire.Error(err.Error())
				}
			default:
				if t.readHandler != nil {
					ok := t.readHandler(fire)
					if !ok {
						fire.Panic("readHandler return false")
						t.Close()
						return
					}
				}
			}
		}
	}
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
	delTopics, err := t.unbindTopic(topics)
	if err != nil {
		return err
	}
	if t.unSubscribeHandler != nil {
		t.unSubscribeHandler(context, delTopics)
	}
	return nil
}

// Publish 推送接口
// 通过BuildTower生成的实例都可以调用该方法来达到推送的目的
func (t *FireTower) Publish(fire *protocol.FireInfo) error {
	if protocol.IsRecycleIn(fire, "") {
		defer tm.brazier.Extinguished(fire)
	}
	err := tm.Publish(fire)
	if err != nil {
		fire.Panic(fmt.Sprintf("publish err: %v", err))
		return err
	}
	return nil
}

// ToSelf 向自己推送消息
// 这里描述一下使用场景
// 只针对当前客户端进行的推送请调用该方法
func (t *FireTower) ToSelf(b []byte) error {
	if t.isClose != true {
		return t.ws.WriteMessage(1, b)
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

func (t *FireTower) SetReceivedHandler(fn func(*protocol.FireInfo) bool) {
	t.receivedHandler = fn
}

// SetReadHandler 客户端推送事件
// 接收到用户publish的消息时触发
func (t *FireTower) SetReadHandler(fn func(*protocol.FireInfo) bool) {
	t.readHandler = fn
}

// SetSubscribeHandler 订阅事件
// 用户订阅topic后触发
func (t *FireTower) SetSubscribeHandler(fn func(context protocol.FireLife, topic []string) bool) {
	t.subscribeHandler = fn
}

// SetUnSubscribeHandler 取消订阅事件
// 用户取消订阅topic后触发
func (t *FireTower) SetUnSubscribeHandler(fn func(context protocol.FireLife, topic []string) bool) {
	t.unSubscribeHandler = fn
}

// SetBeforeSubscribeHandler 订阅前回调事件
// 用户订阅topic前触发
func (t *FireTower) SetBeforeSubscribeHandler(fn func(context protocol.FireLife, topic []string) bool) {
	t.beforeSubscribeHandler = fn
}

// SetReadTimeoutHandler 超时回调
// readIn channal写满了  生产 > 消费的情况下触发超时机制
func (t *FireTower) SetReadTimeoutHandler(fn func(*protocol.FireInfo)) {
	t.readTimeoutHandler = fn
}

// SetOnSystemRemove 系统移除某个用户的topic订阅
func (t *FireTower) SetOnSystemRemove(fn func(topic string)) {
	t.onSystemRemove = fn
}

// GetConnectNum 获取话题订阅数的grpc方法封装
func (t *FireTower) GetConnectNum(topic string) int64 {
	number, err := tm.stores.ClusterTopicStore().GetTopicConnNum(topic)
	if err != nil {
		// todo log & warning
		return 0
	}
	return number
}