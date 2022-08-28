package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	pb "github.com/holdno/firetower/grpc/manager"
	"github.com/holdno/firetower/socket"

	"github.com/gorilla/websocket"
	"google.golang.org/grpc"
)

var (
	topicManage     *socket.TcpClient
	TopicManageGrpc pb.TopicServiceClient
)

// 接收的消息结构体
type FireInfo struct {
	MessageType int
	Data        *TopicMessage
}

type TopicMessage struct {
	Topic string          `json:"topic"`
	Data  json.RawMessage `json:"data"` // 可能是个json
	Type  string          `json:"type"`
}

// 发送的消息结构体
// 发送不用限制用户消息内容的格式
type SendMessage struct {
	MessageType int
	Data        []byte
	Topic       string
}

type FireTower struct {
	connId   uint64 // 连接id 每台服务器上该id从1开始自增
	ClientId string // 客户端id 用来做业务逻辑
	UserId   string // 一般业务中每个连接都是一个用户 用来给业务提供用户识别
	Cookie   []byte // 这里提供给业务放一个存放跟当前连接相关的数据信息

	readIn      chan *FireInfo    // 读取队列
	sendOut     chan *SendMessage // 发送队列
	ws          *websocket.Conn   // 保存底层websocket连接
	Topic       []string          // 订阅topic列表
	isClose     bool              // 判断当前websocket是否被关闭
	closeSwitch chan struct{}     // 用来作为关闭websocket的触发点
	mutex       sync.Mutex        // 避免并发close chan

	readHandler            func(*TopicMessage) bool
	subscribeHandler       func(topic []string) bool
	unSubscribeHandler     func(topic []string) bool
	beforeSubscribeHandler func(topic []string) bool
}

func init() {
	loadConfig()       // 加载配置
	BuildTopicManage() // 构建服务架构

	go func() {

	Retry:
		var err error
		conn, err := grpc.Dial(ConfigTree.Get("grpc.address").(string), grpc.WithInsecure())
		if err != nil {
			fmt.Println("[topic manager] grpc connect error:", ConfigTree.Get("topicServiceAddr").(string), err)
		}
		TopicManageGrpc = pb.NewTopicServiceClient(conn)
		topicManage = socket.NewClient(ConfigTree.Get("topicServiceAddr").(string))

		if err != nil {
			panic(fmt.Sprintf("[topic manager] can not get local IP, error:%v", err))
		}
		topicManage.OnPush(func(t, topic string, message []byte) {
			fmt.Println("[topic manager] on push", "topic:", string(topic), "data:", message)

			TM.centralChan <- &SendMessage{
				MessageType: 1,
				Data:        message,
				Topic:       topic,
			}
		})
		err = topicManage.Connect()
		if err != nil {
			fmt.Println("[topic manager] wait topic manager online", ConfigTree.Get("topicServiceAddr").(string))
			time.Sleep(time.Duration(1) * time.Second)
			goto Retry
		} else {
			fmt.Println("[topic manager] connected:", ConfigTree.Get("topicServiceAddr").(string))
		}
	}()

}

func BuildTower(ws *websocket.Conn, clientId string) (tower *FireTower) {
	tower = &FireTower{
		connId:      getConnId(),
		ClientId:    clientId,
		readIn:      make(chan *FireInfo, ConfigTree.Get("chanLens").(int64)),
		sendOut:     make(chan *SendMessage, ConfigTree.Get("chanLens").(int64)),
		ws:          ws,
		isClose:     false,
		closeSwitch: make(chan struct{}),
	}
	return
}

func (t *FireTower) Run() {
	t.LogInfo(fmt.Sprintf("new websocket running, ConnId:%d ClientId:%s", t.connId, t.ClientId))
	// 读取websocket信息
	go t.readLoop()
	// 处理读取事件
	go t.readDispose()
	// 向websocket发送信息
	t.sendLoop()
}

func (t *FireTower) BindTopic(topic []string) bool {
	var (
		exist    = 0
		addTopic []string
	)
	bucket := TM.GetBucket(t)
	for _, v := range topic {
		for _, vv := range t.Topic {
			if v == vv {
				exist = 1
				break
			}
		}
		if exist == 1 {
			exist = 0
		} else {
			addTopic = append(addTopic, v) // 待订阅的topic
			t.Topic = append(t.Topic, v)
			bucket.AddSubscribe(v, t)
		}
	}
	if len(addTopic) > 0 {
		_, err := TopicManageGrpc.SubscribeTopic(context.Background(), &pb.SubscribeTopicRequest{Topic: addTopic, Ip: topicManage.Conn.LocalAddr().String()})
		if err != nil {
			// 订阅失败影响客户端正常业务逻辑 直接关闭连接
			t.Close()
		} else {
			if t.subscribeHandler != nil {
				t.subscribeHandler(addTopic)
			}
		}
	}
	return true
}

func (t *FireTower) UnbindTopic(topic []string) bool {
	var delTopic []string // 待取消订阅的topic列表
	bucket := TM.GetBucket(t)
	for _, v := range topic {
		for k, vv := range t.Topic {
			if v == vv { // 如果客户端已经订阅过该topic才执行退订
				delTopic = append(delTopic, v)
				t.Topic = append(t.Topic[:k], t.Topic[k+1:]...)
				bucket.DelSubscribe(v, t)
				break
			}
		}
	}
	if len(delTopic) > 0 {
		_, err := TopicManageGrpc.UnSubscribeTopic(context.Background(), &pb.UnSubscribeTopicRequest{Topic: delTopic, Ip: topicManage.Conn.LocalAddr().String()})
		if err != nil {
			// 订阅失败影响客户端正常业务逻辑 直接关闭连接
			t.Close()
		} else {
			if t.subscribeHandler != nil {
				t.unSubscribeHandler(delTopic)
			}
		}
	}
	return true
}

func (t *FireTower) read() (*FireInfo, error) {
	if t.isClose {
		return nil, ErrorClose
	}
	message := <-t.readIn
	return message, nil
}

func (t *FireTower) Send(message *SendMessage) error {
	if t.isClose {
		return ErrorClose
	}
	t.sendOut <- message
	return nil
}

func (t *FireTower) Close() {
	t.LogInfo("websocket connect is closed")
	t.mutex.Lock()
	defer t.mutex.Unlock()
	if !t.isClose {
		t.isClose = true
		if t.Topic != nil {
			fmt.Println("unbindtopic")
			t.UnbindTopic(t.Topic)
		}
		t.ws.Close()
		close(t.closeSwitch)
	}
}

func (t *FireTower) sendLoop() {
	heartTicker := time.NewTicker(time.Duration(ConfigTree.Get("heartbeat").(int64)) * time.Second)
	for {
		select {
		case message := <-t.sendOut:
			if err := t.ws.WriteMessage(message.MessageType, []byte(message.Data)); err != nil {
				goto collapse
			}
		case <-heartTicker.C:
			message := &SendMessage{
				MessageType: websocket.TextMessage,
				Data:        []byte("heartbeat"),
			}
			if err := t.Send(message); err != nil {
				fmt.Println("heartbeat send failed:", err)
				goto collapse
				return
			}
		case <-t.closeSwitch:
			heartTicker.Stop()
			return
		}
	}
collapse:
	t.Close()
}

func (t *FireTower) readLoop() {
	for {
		messageType, data, err := t.ws.ReadMessage() // 内部声明可以及时释放内存
		if err != nil {
			t.LogError(fmt.Sprintf("读取客户端消息时发生错误:%v", err))
			goto collapse // 出现问题烽火台直接坍塌
		}
		t.LogInfo(fmt.Sprintf("new message:%s", string(data)))

		var jsonStruct = new(TopicMessage)

		if err := json.Unmarshal(data, &jsonStruct); err != nil {
			t.LogError(fmt.Sprintf("客户端消息解析错误:%v", err))
			continue
		}

		message := &FireInfo{
			MessageType: messageType,
			Data:        jsonStruct,
		}
		select {
		case t.readIn <- message:
		case <-t.closeSwitch:
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
		message, err := t.read()
		if err != nil {
			t.LogError(fmt.Sprintf("read message failed:%v", err))
			t.Close()
			return
		}
		if err == nil {
			switch message.Data.Type {
			case "subscribe": // 客户端订阅topic
				if message.Data.Topic == "" {
					t.LogError(fmt.Sprintf("onSubscribe:topic is empty, ClintId:%s, UserId:%s", t.ClientId, t.UserId))
					continue
				}
				addTopic := strings.Split(message.Data.Topic, ",")
				// 如果设置了订阅前触发事件则调用
				if t.beforeSubscribeHandler != nil {
					ok := t.beforeSubscribeHandler(addTopic)
					if !ok {
						continue
					}
				}
				t.BindTopic(addTopic)
			case "unSubscribe": // 客户端取消订阅topic
				if message.Data.Topic == "" {
					t.LogError(fmt.Sprintf("unOnSubscribe:topic is empty, ClintId:%s, UserId:%s", t.ClientId, t.UserId))
					continue
				}
				delTopic := strings.Split(message.Data.Topic, ",")
				t.UnbindTopic(delTopic)
			default:
				if t.isClose {
					return
				}
				if t.readHandler != nil {
					ok := t.readHandler(message.Data)
					if !ok {
						t.Close()
						return
					}
				}
			}
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

func (t *FireTower) Publish(message *TopicMessage) bool {
	topicManage.Publish(message.Topic, message.Data)
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
	return true
}

// 接收到用户publish的消息时触发
func (t *FireTower) SetReadHandler(fn func(*TopicMessage) bool) {
	t.readHandler = fn
}

// 用户订阅topic后触发
func (t *FireTower) SetSubscribeHandler(fn func(topic []string) bool) {
	t.subscribeHandler = fn
}

// 用户取消订阅topic后触发
func (t *FireTower) SetUnSubscribeHandler(fn func(topic []string) bool) {
	t.unSubscribeHandler = fn
}

// 用户订阅topic前触发
func (t *FireTower) SetBeforeSubscribeHandler(fn func(topic []string) bool) {
	t.beforeSubscribeHandler = fn
}

// grpc方法封装
func (t *FireTower) GetConnectNum(topic string) int64 {
	res, err := TopicManageGrpc.GetConnectNum(context.Background(), &pb.GetConnectNumRequest{Topic: topic})
	if err != nil {
		return 0
	}
	return res.Number
}
