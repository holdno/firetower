package socket

import (
	"encoding/json"
	"fmt"
	"net"
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
	readIn    chan []byte
	sendOut   chan []byte
}

type TopicEvent struct {
	Topic []string `json:"topic"`
	DATA  []byte   `json:"data"`
	Type  string   `json:"type"`
}

type PushMessage struct {
	Topic string
	Data  []byte
}

func NewClient(address string) *TcpClient {
	return &TcpClient{
		Address:   address,
		closeChan: make(chan struct{}),
		isClose:   false,
		readIn:    make(chan []byte, 1024),
		sendOut:   make(chan []byte, 1024),
	}
}

func (t *TcpClient) Connect() error {
	lis, err := net.Dial("tcp", t.Address)
	if err != nil {
		return err
	}
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
		for {
			var msg = make([]byte, 1024)
			l, err := lis.Read(msg)
			if err != nil {
				if !t.isClose {
					t.Close()
				}
				return
			}
			select {
			case t.readIn <- msg[:l]:
			case <-t.closeChan:
				if !t.isClose {
					t.Close()
					return
				}
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

func (t *TcpClient) Read() ([]byte, error) {
	if t.isClose {
		return nil, ErrorClose
	}
Retry:
	message := <-t.readIn
	if string(message) == "heartbeat" {
		goto Retry
	}
	return message, nil
}

func (t *TcpClient) send(message []byte) error {
	if t.isClose {
		return ErrorClose
	}
	// 设置一秒超时
	ticker := time.NewTicker(time.Duration(1) * time.Second)
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

func (t *TcpClient) Publish(topic string, data string) error {
	// 发消息只能发送一条
	message := &TopicEvent{
		Topic: []string{topic},
		Type:  PublishKey,
		DATA:  []byte(data),
	}

	if data, err := json.Marshal(message); err == nil {
		t.send(data)
		return nil
	} else {
		return err
	}
}

func (t *TcpClient) OnPush(fn func(topic string, message []byte)) {
	go func() {
		for {
			message, err := t.Read()
			if err != nil {
				fmt.Println(err)
				// 只可能是连接断开了
				return
			}
			data := new(PushMessage)
			err = json.Unmarshal(message, &data)
			if err != nil {
				// 消息下发格式不统一 直接忽略
				fmt.Sprintf("push message format error:%v", err)
				continue
			}
			fn(data.Topic, data.Data)
		}
	}()
}
