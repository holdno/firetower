package socket

import (
	"fmt"
	json "github.com/json-iterator/go"
	"net"
	"sync"
	"time"
)

const (
	SubKey     = "subscribe"
	UnSubKey   = "unSubscribe"
	PublishKey = "publish"
)

type TcpClient struct {
	Address   string
	isClose   bool
	closeChan chan struct{}
	Conn      net.Conn
	readIn    chan *SendMessage
	sendOut   chan []byte
}

type TopicEvent struct {
	Topic []string        `json:"topic"`
	DATA  json.RawMessage `json:"data"`
	Type  string          `json:"type"`
}

type PushMessage struct {
	MessageId string `json:"message_id"`
	Source    string `json:"source"`
	Topic     string `json:"topic"`
	Data      []byte `json:"data"`
	Type      string `json:"type"`
}

// 发送的消息结构体
// 发送不用限制用户消息内容的格式
type SendMessage struct {
	Context     *sendLife
	Type        string
	MessageType int
	Data        []byte
	Topic       string
}

type sendLife struct {
	StartTime time.Time
	Id        string
	Source    string
}

var (
	sendPool sync.Pool
)

// id 记录消息id
// source 记录这条信息是用户推送还是平台推送 user | platform
func GetSendMessage(id, source string) *SendMessage {
	sendMessage := sendPool.Get().(*SendMessage)
	sendMessage.Context.StartTime = time.Now()
	sendMessage.Context.Id = id
	sendMessage.Context.Source = source
	return sendMessage
}

func init() {
	sendPool.New = func() interface{} {
		return &SendMessage{
			Context: new(sendLife),
		}
	}
	SendLogger = sendLog
}

func NewClient(address string) *TcpClient {
	return &TcpClient{
		Address: address,
		isClose: false,
		readIn:  make(chan *SendMessage, 1024),
		sendOut: make(chan []byte, 1024),
	}
}

func (t *TcpClient) Connect() error {
	lis, err := net.Dial("tcp", t.Address)
	if err != nil {
		return err
	}
	t.isClose = false
	t.closeChan = make(chan struct{})
	t.Conn = lis

	go func() {
		for {
			select {
			case message := <-t.sendOut:
				if _, err := lis.Write(message); err != nil {
					goto close
				}
			case <-t.closeChan:
				return
			}
		}
	close:
		t.Close()
	}()

	// read channal
	go func() {
		var overflow []byte
		for {
			var msg = make([]byte, 1024*16)

			l, err := lis.Read(msg)
			if err != nil {
				if !t.isClose {
					t.Close()
				}
				return
			}
			overflow, err = Depack(append(overflow, msg[:l]...), t.readIn)
			if err != nil {
				fmt.Println("[manager client] depack error:", err)
			}
			select {
			case <-t.closeChan:
				if !t.isClose {
					t.Close()
					return
				}
			default:
			}
		}
	}()

	return nil
}

func (t *TcpClient) Close() {
	if !t.isClose {
		fmt.Println("socket close")
		t.isClose = true
		t.Conn.Close()
		close(t.closeChan)
	Retry:
		err := t.Connect()
		if err != nil {
			fmt.Println("[topic manager] wait topic manager online", t.Address)
			time.Sleep(time.Duration(1) * time.Second)
			goto Retry
		} else {
			fmt.Println("[topic manager] connected:", t.Address)
		}
	}
}

func (t *TcpClient) Read() (*SendMessage, error) {
	if t.isClose {
		return nil, ErrorClose
	}
Retry:
	message := <-t.readIn
	if string(message.Type) == "heartbeat" {
		goto Retry
	}
	return message, nil
}

func (t *TcpClient) send(message []byte) error {
	if t.isClose {
		return ErrorClose
	}
	// 设置一秒超时
	ticker := time.NewTicker(time.Duration(3) * time.Second)
	for {
		select {
		case t.sendOut <- message:
			ticker.Stop()
			return nil
		case <-ticker.C:
			fmt.Println("[topic manager] send timeout:", message)
			ticker.Stop()
			return ErrorBlock
		}
	}
}

func (t *TcpClient) AddTopic(topic []string) error {
	message := &TopicEvent{
		Topic: topic,
		Type:  SubKey,
	}

	if data, err := json.Marshal(message); err == nil {
		t.send(data)
		return nil
	} else {
		return err
	}
}

func (t *TcpClient) DelTopic(topic []string) error {
	message := &TopicEvent{
		Topic: topic,
		Type:  UnSubKey,
	}

	if data, err := json.Marshal(message); err == nil {
		t.send(data)
		return nil
	} else {
		return err
	}
}

func (t *TcpClient) Publish(messageId, source, topic string, data json.RawMessage) error {
	b, err := Enpack(PublishKey, messageId, source, topic, data)
	if err != nil {
		return err
	}
	return t.send(b)
}

func (t *TcpClient) OnPush(fn func(message *SendMessage)) {
	go func() {
		for {
			message, err := t.Read()
			if err != nil {
				message.Panic(err.Error())
				// 只可能是连接断开了
				return
			}
			fn(message)
		}
	}()
}
