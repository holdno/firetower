package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	pb "github.com/holdno/firetower/grpc/manager"
	"github.com/holdno/firetower/socket"
	"github.com/holdno/snowFlakeByGo"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	topicManage     *socket.TcpClient
	TopicManageGrpc pb.TopicServiceClient

	ClusterId = 1

	// 默认配置文件读取路径
	DefaultConfigPath = "./fireTower.toml"
	TowerLogger       func(t *FireTower, types, info string) // 接管系统log t log类型 info log信息
	FireLogger        func(f *FireInfo, types, info string)  // 接管系统log t log类型 info log信息

	firePool sync.Pool

	IdWorker *snowFlakeByGo.Worker
)

// 接收的消息结构体
type FireInfo struct {
	Context     *FireLife
	MessageType int
	Message     *TopicMessage
}

type TopicMessage struct {
	Topic string          `json:"topic"`
	Data  json.RawMessage `json:"data"` // 可能是个json
	Type  string          `json:"type"`
}

// 第二个参数的作用是继承
// 继承上一个消息体的上下文，方便日志追踪或逻辑统一
func NewFireInfo(t *FireTower, context *FireLife) *FireInfo {
	fireInfo := firePool.Get().(*FireInfo)
	if context != nil {
		fireInfo.Context = context
	} else {
		fireInfo.Context.reset(t)
	}
	return fireInfo
}

func (f *FireInfo) Recycling() {
	firePool.Put(f)
}

func (f *FireInfo) Panic(info string) {
	FireLogger(f, "Panic", info)
	f.Recycling()
}

func (f *FireInfo) Info(info string) {
	FireLogger(f, "INFO", info)
}

func (f *FireInfo) Error(info string) {
	FireLogger(f, "ERROR", info)
}

type FireLife struct {
	id        string
	startTime time.Time
	message   *TopicMessage
	clientId  string
	userId    string
}

func (f *FireLife) reset(t *FireTower) {
	f.startTime = time.Now()
	f.id = strconv.FormatInt(IdWorker.GetId(), 10)
	f.clientId = t.ClientId
	f.userId = t.UserId
}

type FireTower struct {
	connId    uint64 // 连接id 每台服务器上该id从1开始自增
	ClientId  string // 客户端id 用来做业务逻辑
	UserId    string // 一般业务中每个连接都是一个用户 用来给业务提供用户识别
	Cookie    []byte // 这里提供给业务放一个存放跟当前连接相关的数据信息
	startTime time.Time

	readIn    chan *FireInfo           // 读取队列
	sendOut   chan *socket.SendMessage // 发送队列
	ws        *websocket.Conn          // 保存底层websocket连接
	Topic     map[string]bool          // 订阅topic列表
	isClose   bool                     // 判断当前websocket是否被关闭
	closeChan chan struct{}            // 用来作为关闭websocket的触发点
	mutex     sync.Mutex               // 避免并发close chan

	readHandler            func(*FireInfo) bool
	readTimeoutHandler     func(*FireInfo)
	subscribeHandler       func(context *FireLife, topic []string) bool
	unSubscribeHandler     func(context *FireLife, topic []string) bool
	beforeSubscribeHandler func(context *FireLife, topic []string) bool
}

type Context struct {
	startTime time.Time
}

func init() {
	firePool.New = func() interface{} {
		return &FireInfo{
			Context: new(FireLife),
			Message: new(TopicMessage),
		}
	}

	IdWorker, _ = snowFlakeByGo.NewWorker(1)

	TowerLogger = towerLog
	FireLogger = fireLog

	loadConfig(DefaultConfigPath) // 加载配置
	buildBuckets()                // 构建服务架构
	BuildManagerClient()          // 构建连接manager(topic管理服务)的客户端
}

func BuildTower(ws *websocket.Conn, clientId string) (tower *FireTower) {
	tower = &FireTower{
		connId:    getConnId(),
		ClientId:  clientId,
		startTime: time.Now(),
		readIn:    make(chan *FireInfo, ConfigTree.Get("chanLens").(int64)),
		sendOut:   make(chan *socket.SendMessage, ConfigTree.Get("chanLens").(int64)),
		Topic:     make(map[string]bool),
		ws:        ws,
		isClose:   false,
		closeChan: make(chan struct{}),
	}
	return
}

func (t *FireTower) Run() {
	logInfo(t, "new websocket running")
	// 读取websocket信息
	go t.readLoop()
	// 处理读取事件
	go t.readDispose()
	// 向websocket发送信息
	t.sendLoop()
}

func (t *FireTower) BindTopic(topic []string) (error, []string) {
	var (
		addTopic []string
	)
	bucket := TM.GetBucket(t)
	for _, v := range topic {
		if _, ok := t.Topic[v]; !ok {
			addTopic = append(addTopic, v) // 待订阅的topic
			t.Topic[v] = true
			bucket.AddSubscribe(v, t)
		}
	}
	if len(addTopic) > 0 {
		_, err := TopicManageGrpc.SubscribeTopic(context.Background(), &pb.SubscribeTopicRequest{Topic: addTopic, Ip: topicManage.Conn.LocalAddr().String()})
		if err != nil {
			// 订阅失败影响客户端正常业务逻辑 直接关闭连接
			t.Close()
			return err, addTopic
		}
	}
	return nil, addTopic
}

func (t *FireTower) UnbindTopic(topic []string) (error, []string) {
	var delTopic []string // 待取消订阅的topic列表
	bucket := TM.GetBucket(t)
	for _, v := range topic {
		if _, ok := t.Topic[v]; ok {
			// 如果客户端已经订阅过该topic才执行退订
			delTopic = append(delTopic, v)
			delete(t.Topic, v)
			bucket.DelSubscribe(v, t)
		}
	}
	if len(delTopic) > 0 {
		_, err := TopicManageGrpc.UnSubscribeTopic(context.Background(), &pb.UnSubscribeTopicRequest{Topic: delTopic, Ip: topicManage.Conn.LocalAddr().String()})
		if err != nil {
			// 订阅失败影响客户端正常业务逻辑 直接关闭连接
			t.Close()
			return err, delTopic
		}
	}
	return nil, delTopic
}

func (t *FireTower) read() (*FireInfo, error) {
	if t.isClose {
		return nil, ErrorClose
	}
	message := <-t.readIn
	return message, nil
}

func (t *FireTower) Send(message *socket.SendMessage) error {
	if t.isClose {
		return ErrorClose
	}
	t.sendOut <- message
	return nil
}

func (t *FireTower) Close() {
	logInfo(t, "websocket connect is closed")
	t.mutex.Lock()
	defer t.mutex.Unlock()
	if !t.isClose {
		t.isClose = true
		if t.Topic != nil {
			var topicSlice []string
			for k := range t.Topic {
				topicSlice = append(topicSlice, k)
			}
			t.UnbindTopic(topicSlice)
		}
		t.ws.Close()
		close(t.closeChan)
	}
}

func (t *FireTower) sendLoop() {
	heartTicker := time.NewTicker(time.Duration(ConfigTree.Get("heartbeat").(int64)) * time.Second)
	for {
		select {
		case message := <-t.sendOut:
			if message.MessageType == 0 {
				message.MessageType = 1 // 文本格式
			}
			if err := t.ws.WriteMessage(message.MessageType, []byte(message.Data)); err != nil {
				message.Panic(err.Error())
				goto collapse
			}
		case <-heartTicker.C:
			sendMessage := socket.GetSendMessage("0", "system")
			sendMessage.MessageType = websocket.TextMessage
			sendMessage.Data = []byte("heartbeat")
			if err := t.Send(sendMessage); err != nil {
				goto collapse
				return
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
	for {
		messageType, data, err := t.ws.ReadMessage()
		if err != nil { // 断开连接
			goto collapse // 出现问题烽火台直接坍塌
		}
		fire := NewFireInfo(t, nil) // 从对象池中获取消息对象 降低GC压力
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
		if err == nil {
			switch fire.Message.Type {
			case "subscribe": // 客户端订阅topic
				if fire.Message.Topic == "" {
					fire.Panic(fmt.Sprintf("onSubscribe:topic is empty, ClintId:%s, UserId:%s", t.ClientId, t.UserId))
					continue
				}
				addTopic := strings.Split(fire.Message.Topic, ",")
				// 如果设置了订阅前触发事件则调用
				if t.beforeSubscribeHandler != nil {
					ok := t.beforeSubscribeHandler(fire.Context, addTopic)
					if !ok {
						continue
					}
				}
				// 增加messageId 方便追踪
				err, addTopic = t.BindTopic(addTopic)
				if err != nil {
					fire.Error(err.Error())
				} else {
					if t.subscribeHandler != nil {
						t.subscribeHandler(fire.Context, addTopic)
					}
				}
			case "unSubscribe": // 客户端取消订阅topic
				if fire.Message.Topic == "" {
					fire.Panic(fmt.Sprintf("unOnSubscribe:topic is empty, ClintId:%s, UserId:%s", t.ClientId, t.UserId))
					continue
				}
				delTopic := strings.Split(fire.Message.Topic, ",")
				err, delTopic = t.UnbindTopic(delTopic)
				if err != nil {
					fire.Error(err.Error())
				} else {
					if t.unSubscribeHandler != nil {
						t.unSubscribeHandler(fire.Context, delTopic)
					}
				}
			default:
				if t.isClose {
					return
				}
				if t.readHandler != nil {
					ok := t.readHandler(fire)
					if !ok {
						t.Close()
						return
					}
				}
			}
			fire.Info("Extinguished")
			fire.Recycling()
		} else {
			// TODO 正常情况下不会出现无法json序列化的情况 因为这个参数本身就是string to json来的
		}
	}
}

//func (t *FireTower) Event(event, topic, message string) {
//	switch event {
//	case socket.SubKey:
//		if t.subscribeHandler != nil {
//			t.subscribeHandler(topic)
//		}
//	case socket.UnSubKey:
//		if t.subscribeHandler != nil {
//			t.unSubscribeHandler(topic, message)
//		}
//	}
//}

func (t *FireTower) Publish(fire *FireInfo) error {
	err := topicManage.Publish(fire.Context.id, "user", fire.Message.Topic, fire.Message.Data)
	if err != nil {
		fire.Panic(fmt.Sprintf("publish err: %v", err))
		return err
	}
	//res, err := TopicManageGrpc.Publish(context.Background(), &pb.PublishRequest{
	//	Topic: message.Topic,
	//	Data:  message.Data,
	//})
	//if err != nil {
	//	t.LogError(fmt.Sprintf("Publish Error, Topic:%s,Data:%s; error:%v", message.Topic, string(message.Data), err))
	//	return false
	//} else {
	//	// TODO 发送失败
	//	return res.Ok
	//}
	return nil
}

func (t *FireTower) ToSelf(b []byte) error {
	if t.isClose != true {
		err := t.ws.WriteMessage(1, b)
		return err
	} else {
		return ErrorClose
	}
}

// 接收到用户publish的消息时触发
func (t *FireTower) SetReadHandler(fn func(*FireInfo) bool) {
	t.readHandler = fn
}

// 用户订阅topic后触发
func (t *FireTower) SetSubscribeHandler(fn func(context *FireLife, topic []string) bool) {
	t.subscribeHandler = fn
}

// 用户取消订阅topic后触发
func (t *FireTower) SetUnSubscribeHandler(fn func(context *FireLife, topic []string) bool) {
	t.unSubscribeHandler = fn
}

// 用户订阅topic前触发
func (t *FireTower) SetBeforeSubscribeHandler(fn func(context *FireLife, topic []string) bool) {
	t.beforeSubscribeHandler = fn
}

// readIn channal写满了  生产 > 消费的情况下触发超时机制
func (t *FireTower) SetReadTimeoutHandler(fn func(*FireInfo)) {
	t.readTimeoutHandler = fn
}

// grpc方法封装
func (t *FireTower) GetConnectNum(topic string) int64 {
	res, err := TopicManageGrpc.GetConnectNum(context.Background(), &pb.GetConnectNumRequest{Topic: topic})
	if err != nil {
		return 0
	}
	return res.Number
}
