package gateway

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	pb "github.com/OSMeteor/firetower/grpc/manager"
	"github.com/OSMeteor/firetower/socket"
	"github.com/holdno/snowFlakeByGo"
	json "github.com/json-iterator/go"
)

var (
	topicManage *socket.TcpClient
	// topicManageGrpc 话题管理服务的grpc客户端
	topicManageGrpc pb.TopicServiceClient
	// ClusterId 当前实例在集群中的唯一id
	ClusterId int64 = 1

	// DefaultConfigPath 默认配置文件读取路径
	DefaultConfigPath = "./fireTower.toml"
	// TowerLogger 接管系统log t log类型 info log信息
	TowerLogger func(t *FireTower, types, info string)
	// FireLogger 接管链接log t log类型 info log信息
	FireLogger func(f *FireInfo, types, info string)

	towerPool sync.Pool
	firePool  sync.Pool
	// IdWorker 全局唯一id生成器实例
	IdWorker *snowFlakeByGo.Worker
)
func GetTopicManage() *socket.TcpClient{ 
	return topicManage
}
func GetTopicManageGrpc() pb.TopicServiceClient{
	return topicManageGrpc
}
// FireInfo 接收的消息结构体
type FireInfo struct {
	Context     *FireLife
	MessageType int
	Message     *TopicMessage
}

// TopicMessage 话题信息结构体
type TopicMessage struct {
	Topic string          `json:"topic"`
	Data  json.RawMessage `json:"data"` // 可能是个json
	Type  string          `json:"type"`
}

// NewFireInfo 第二个参数的作用是继承
// 继承上一个消息体的上下文，方便日志追踪或逻辑统一
func NewFireInfo(t *FireTower, context *FireLife) *FireInfo {
	fireInfo := firePool.Get().(*FireInfo)
	if context != nil {
	  if(fireInfo!=nil && fireInfo.Context!=nil){
      fireInfo.Context = context
   	}else {
      return fireInfo
    }
	} else {
		fireInfo.Context.reset(t)
	}
	return fireInfo
}


// Recycling 变量回收
func (f *FireInfo) Recycling() {
	firePool.Put(f)
}

// Panic 消息的panic日志 并回收变量
func (f *FireInfo) Panic(info string) {
	FireLogger(f, "Panic", info)
	f.Recycling()
}

// Info 记录一个INFO级别的日志
func (f *FireInfo) Info(info string) {
	FireLogger(f, "INFO", info)
}

// Error 记录一个ERROR级别的日志
func (f *FireInfo) Error(info string) {
	FireLogger(f, "ERROR", info)
}

// FireLife 客户端推送消息的结构体
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

// FireTower 客户端连接结构体
// 包含了客户端一个连接的所有信息
type FireTower struct {
	connId    uint64 // 连接id 每台服务器上该id从1开始自增
	ClientId  string // 客户端id 用来做业务逻辑
	UserId    string // 一般业务中每个连接都是一个用户 用来给业务提供用户识别
	Cookie    []byte // 这里提供给业务放一个存放跟当前连接相关的数据信息
	startTime time.Time

	readIn    chan *FireInfo           // 读取队列
	sendOut   chan *socket.SendMessage // 发送队列
	ws        *websocket.Conn          // 保存底层websocket连接
	topic     map[string]bool          // 订阅topic列表
	isClose   bool                     // 判断当前websocket是否被关闭
	closeChan chan struct{}            // 用来作为关闭websocket的触发点
	mutex     sync.Mutex               // 避免并发close chan

	onConnectHandler       func() bool
	onOfflineHandler       func()
	readHandler            func(*FireInfo) bool
	readTimeoutHandler     func(*FireInfo)
	subscribeHandler       func(context *FireLife, topic []string) bool
	unSubscribeHandler     func(context *FireLife, topic []string) bool
	beforeSubscribeHandler func(context *FireLife, topic []string) bool
	onSystemRemove         func(topic string)
}

// Init 初始化firetower
// 在调用firetower前请一定要先调用Init方法
func Init() {
	towerPool.New = func() interface{} {
		return &FireTower{}
	}

	firePool.New = func() interface{} {
		return &FireInfo{
			Context: new(FireLife),
			Message: new(TopicMessage),
		}
	}

	IdWorker, _ = snowFlakeByGo.NewWorker(ClusterId)

	TowerLogger = towerLog
	FireLogger = fireLog

	loadConfig(DefaultConfigPath) // 加载配置
	buildBuckets()                // 构建服务架构
	buildManagerClient()          // 构建连接manager(topic管理服务)的客户端
}

// BuildTower 实例化一个websocket客户端
func BuildTower(ws *websocket.Conn, clientId string) (tower *FireTower) {
	if topicManageGrpc == nil {
		panic("please confirm gateway was inited")
	}

	if ws == nil {
		towerLog(tower, "ERROR", "websocket.Conn is nil")
		return
	}
	tower = buildNewTower(ws, clientId)
	return
}

func buildNewTower(ws *websocket.Conn, clientId string) *FireTower {
	t := towerPool.Get().(*FireTower)
	t.connId = getConnId()
	t.ClientId = clientId
	t.startTime = time.Now()
	t.readIn = make(chan *FireInfo, ConfigTree.Get("chanLens").(int64))
	t.sendOut = make(chan *socket.SendMessage, ConfigTree.Get("chanLens").(int64))
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

// Run 启动websocket客户端
func (t *FireTower) Run() {
	logInfo(t, "new websocket running")
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

// 订阅topic的绑定过程
func (t *FireTower) bindTopic(topic []string) ([]string, error) {
	var (
		addTopic []string
	)
	bucket := TM.GetBucket(t)
	for _, v := range topic {
		if _, ok := t.topic[v]; !ok {
			addTopic = append(addTopic, v) // 待订阅的topic
			t.topic[v] = true
			bucket.AddSubscribe(v, t)
		}
	}
	if len(addTopic) > 0 {
		_, err := topicManageGrpc.SubscribeTopic(context.Background(), &pb.SubscribeTopicRequest{Topic: addTopic, Ip: topicManage.Conn.LocalAddr().String()})
		if err != nil {
			// 订阅失败影响客户端正常业务逻辑 直接关闭连接
			t.Close()
			return addTopic, err
		}
	}
	return addTopic, nil
}

func (t *FireTower) unbindTopic(topic []string) ([]string, error) {
	var delTopic []string // 待取消订阅的topic列表
	bucket := TM.GetBucket(t)
	for _, v := range topic {
		if _, ok := t.topic[v]; ok {
			// 如果客户端已经订阅过该topic才执行退订
			delTopic = append(delTopic, v)
			delete(t.topic, v)
			bucket.DelSubscribe(v, t)
		}
	}
	if len(delTopic) > 0 {
		_, err := topicManageGrpc.UnSubscribeTopic(context.Background(), &pb.UnSubscribeTopicRequest{Topic: delTopic, Ip: topicManage.Conn.LocalAddr().String()})
		if err != nil {
			// 订阅失败影响客户端正常业务逻辑 直接关闭连接
			t.Close()
			return delTopic, err
		}
	}
	return delTopic, nil
}

func (t *FireTower) read() (*FireInfo, error) {
	if t.isClose {
		return nil, ErrorClose
	}
	message := <-t.readIn
	return message, nil
}

// Send 发送消息方法
// 向某个topic发送某段信息
func (t *FireTower) Send(message *socket.SendMessage) error {
	if t.isClose {
		return ErrorClose
	}
	t.sendOut <- message
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
			fire := NewFireInfo(t, nil)
			if err != nil {
				fire.Panic(err.Error())
			} else {
				if t.unSubscribeHandler != nil {
					t.unSubscribeHandler(fire.Context, delTopic)
				}
			}
			fire.Recycling()
		}
		t.ws.Close()
		close(t.closeChan)
		if t.onOfflineHandler != nil {
			t.onOfflineHandler()
		}
		towerPool.Put(t)
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
			sendMessage.Data = []byte{104, 101, 97, 114, 116, 98, 101, 97, 116} // []byte("heartbeat")
			if err := t.Send(sendMessage); err != nil {
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
		} else if t.isClose {
			return
		} else {
			if fire.Message.Topic == "" {
				fire.Panic(fmt.Sprintf("%s:topic is empty, ClintId:%s, UserId:%s", fire.Message.Type, t.ClientId, t.UserId))
				continue
			}
			switch fire.Message.Type {
			case "subscribe": // 客户端订阅topic
				addTopic := strings.Split(fire.Message.Topic, ",")
				// 如果设置了订阅前触发事件则调用
				if t.beforeSubscribeHandler != nil {
					ok := t.beforeSubscribeHandler(fire.Context, addTopic)
					if !ok {
						continue
					}
				}
				// 增加messageId 方便追踪
				addTopic, err = t.bindTopic(addTopic)
				if err != nil {
					fire.Error(err.Error())
				} else if t.subscribeHandler != nil {
					t.subscribeHandler(fire.Context, addTopic)
				}
			case "unSubscribe": // 客户端取消订阅topic
				delTopic := strings.Split(fire.Message.Topic, ",")
				delTopic, err = t.unbindTopic(delTopic)
				if err != nil {
					fire.Error(err.Error())
				} else if t.unSubscribeHandler != nil {
					t.unSubscribeHandler(fire.Context, delTopic)
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
			fire.Info("Extinguished")
			fire.Recycling()
		}
	}
}

// Publish 推送接口
// 通过BuildTower生成的实例都可以调用该方法来达到推送的目的
func (t *FireTower) Publish(fire *FireInfo) error {
	err := topicManage.Publish(fire.Context.id, "user", fire.Message.Topic, fire.Message.Data)
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

// CheckTopicExist 检测topic是否已经有人订阅
func (t *FireTower) CheckTopicExist(topic string) bool {
	res, err := topicManageGrpc.CheckTopicExist(context.Background(), &pb.CheckTopicExistRequest{Topic: topic})
	if err != nil {
		return false
	}
	return res.Ok
}

// SetOnConnectHandler 建立连接事件
func (t *FireTower) SetOnConnectHandler(fn func() bool) {
	t.onConnectHandler = fn
}

// SetOnOfflineHandler 用户连接关闭时触发
func (t *FireTower) SetOnOfflineHandler(fn func()) {
	t.onOfflineHandler = fn
}

// SetReadHandler 客户端推送事件
// 接收到用户publish的消息时触发
func (t *FireTower) SetReadHandler(fn func(*FireInfo) bool) {
	t.readHandler = fn
}

// SetSubscribeHandler 订阅事件
// 用户订阅topic后触发
func (t *FireTower) SetSubscribeHandler(fn func(context *FireLife, topic []string) bool) {
	t.subscribeHandler = fn
}

// SetUnSubscribeHandler 取消订阅事件
// 用户取消订阅topic后触发
func (t *FireTower) SetUnSubscribeHandler(fn func(context *FireLife, topic []string) bool) {
	t.unSubscribeHandler = fn
}

// SetBeforeSubscribeHandler 订阅前回调事件
// 用户订阅topic前触发
func (t *FireTower) SetBeforeSubscribeHandler(fn func(context *FireLife, topic []string) bool) {
	t.beforeSubscribeHandler = fn
}

// SetReadTimeoutHandler 超时回调
// readIn channal写满了  生产 > 消费的情况下触发超时机制
func (t *FireTower) SetReadTimeoutHandler(fn func(*FireInfo)) {
	t.readTimeoutHandler = fn
}

// SetOnSystemRemove 系统移除某个用户的topic订阅
func (t *FireTower) SetOnSystemRemove(fn func(topic string)) {
	t.onSystemRemove = fn
}

// GetConnectNum 获取话题订阅数的grpc方法封装
func (t *FireTower) GetConnectNum(topic string) int64 {
	res, err := topicManageGrpc.GetConnectNum(context.Background(), &pb.GetConnectNumRequest{Topic: topic})
	if err != nil {
		return 0
	}
	return res.Number
}
