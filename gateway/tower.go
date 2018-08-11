package gateway

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/holdno/beacon/socket"
	"github.com/holdno/beacon/store"
	"strings"
	"sync"
	"time"
)

var (
	topicManage *socket.TcpClient
)

// 接收的消息结构体
type FireInfo struct {
	MessageType int
	Data        *TopicMessage
}

type TopicMessage struct {
	Topic string `json:"topic"`
	Data  string `json:"data"` // 可能是个json
}

// 发送的消息结构体
// 发送不用限制用户消息内容的格式
type SendMessage struct {
	MessageType int
	Data        []byte
}

type BeaconTower struct {
	ClientId string
	UserId   string
	Cookie   []byte

	readIn      chan *FireInfo    // 读取队列
	sendOut     chan *SendMessage // 发送队列
	ws          *websocket.Conn   // 保存底层websocket连接
	Topic       []string          // 订阅topic列表
	isClose     bool              // 判断当前websocket是否被关闭
	closeSwitch chan struct{}     // 用来作为关闭websocket的触发点
	mutex       sync.Mutex        // 避免并发close chan

	readHandler func(*TopicMessage) bool
}

func init() {
	go func() {
	Retry:
		topicManage = socket.NewClient(ConfigTree.Get("topicServiceAddr").(string))
		var err error
		store.IP, err = GetIP()
		if err != nil {
			panic(fmt.Sprintf("[topic manager] can not get local IP, error:%v", err))
		}
		topicManage.OnPush(func(topic string, message []byte) {
			fmt.Println("[topic manager] on push", "topic:", string(topic), "data:", string(message))
			value, ok := store.TopicTable.Load(topic)
			if ok {
				conns := value.([]*BeaconTower)
				for _, v := range conns {
					v.Send(&SendMessage{
						websocket.TextMessage,
						message,
					})
				}
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

func BuildTower(ws *websocket.Conn) (tower *BeaconTower) {
	tower = &BeaconTower{
		readIn:      make(chan *FireInfo, ConfigTree.Get("chanLens").(int64)),
		sendOut:     make(chan *SendMessage, ConfigTree.Get("chanLens").(int64)),
		ws:          ws,
		isClose:     false,
		closeSwitch: make(chan struct{}),
	}
	return
}

func (t *BeaconTower) Run() {
	// 读取websocket信息
	go t.readLoop()
	// 处理读取事件
	go t.readDispose()
	// 向websocket发送信息
	t.sendLoop()
}

func (t *BeaconTower) BindTopic(topic []string) bool {
	var (
		exist    = 0
		addTopic []string
	)

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
			// 先判断当前topic是否存在订阅列表中
			value, ok := store.TopicTable.Load(v)
			if ok {
				valueType := value.([]*BeaconTower)
				valueType = append(valueType, t)
				store.TopicTable.Store(v, valueType)
			} else {
				var valueType []*BeaconTower
				valueType = append(valueType, t)
				store.TopicTable.Store(v, valueType)
			}
			err := topicManage.AddTopic(addTopic)
			if err != nil {
				// 订阅失败影响客户端正常业务逻辑 直接关闭连接
				t.Close()
				return false
			}
		}
	}
	return true
}

func (t *BeaconTower) UnbindTopic(topic []string) bool {
	var delTopic []string // 待取消订阅的topic列表
	for _, v := range topic {
		for _, vv := range t.Topic {
			if v == vv { // 如果客户端已经订阅过该topic才执行退订
				delTopic = append(delTopic, v)
				break
			}
		}
	}
	err := topicManage.DelTopic(delTopic)
	if err != nil {
		// 订阅失败影响客户端正常业务逻辑 直接关闭连接
		t.Close()
		return false
	}
	return true
}

func (t *BeaconTower) read() (*FireInfo, error) {
	if t.isClose {
		return nil, ErrorClose
	}
	message := <-t.readIn
	return message, nil
}

func (t *BeaconTower) Send(message *SendMessage) error {
	if t.isClose {
		return ErrorClose
	}
	t.sendOut <- message
	return nil
}

func (t *BeaconTower) Close() {
	t.LogInfo("websocket connect is closed")
	t.ws.Close()
	t.mutex.Lock()
	defer t.mutex.Unlock()
	if !t.isClose {
		t.isClose = true
		if t.Topic != nil {
			topicManage.DelTopic(t.Topic) // IP目前没什么用
		}
		t.ws.Close()
		close(t.closeSwitch)
	}
}

func (t *BeaconTower) sendLoop() {
	heartTicker := time.NewTicker(time.Duration(ConfigTree.Get("heartbeat").(int64)) * time.Second)
	for {
		select {
		case message := <-t.sendOut:
			if err := t.ws.WriteMessage(message.MessageType, message.Data); err != nil {
				goto collapse
			}
		case <-heartTicker.C:
			message := &SendMessage{
				MessageType: websocket.TextMessage,
				Data:        []byte(ConfigTree.Get("heartbeatContent").(string)),
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

func (t *BeaconTower) readLoop() {
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
func (t *BeaconTower) readDispose() {
	for {
		message, err := t.read()
		if err != nil {
			t.LogError(fmt.Sprintf("read message failed:%v", err))
			continue
		}
		fmt.Println(string(message.Data.Data))
		switch string(message.Data.Data) {
		case "subscribe": // 客户端订阅topic
			if message.Data.Topic == "" {
				t.LogError(fmt.Sprintf("onSubscribe:topic is empty, ClintId:%s, UserId:%s", t.ClientId, t.UserId))
				continue
			}
			addTopic := strings.Split(message.Data.Topic, ",")
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
	}
}

func (t *BeaconTower) Publish(message *TopicMessage) {
	topicManage.Publish(message.Topic, message.Data)
}

func (t *BeaconTower) SetReadHandler(fn func(*TopicMessage) bool) {
	t.readHandler = fn
}
