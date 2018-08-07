package logic

import "sync"

import (
	"fmt"
	"github.com/gorilla/websocket"
	"time"
)

// 消息结构体
type fireInfo struct {
	messageType int
	data        []byte
}

type beaconTower struct {
	readIn      chan *fireInfo  // 读取队列
	sendOut     chan *fireInfo  // 写入队列
	ws          *websocket.Conn // 保存底层websocket连接
	isClose     bool            // 判断当前websocket是否被关闭
	closeSwitch chan struct{}   // 用来作为关闭websocket的触发点
	mutex       sync.Mutex      // 避免并发close chan
}

func BuildTower(ws *websocket.Conn) (tower *beaconTower) {
	tower = &beaconTower{
		readIn:      make(chan *fireInfo, BTConfig.chanLens),
		sendOut:     make(chan *fireInfo, BTConfig.chanLens),
		ws:          ws,
		isClose:     false,
		closeSwitch: make(chan struct{}),
	}

	// 进行业务逻辑处理
	go tower.procLoop()
	// 读取websocket信息
	go tower.readLoop()
	// websocket发送信息
	go tower.sendLoop()

	return
}

func (t *beaconTower) Read() (*fireInfo, error) {
	if t.isClose {
		return nil, ErrorClose
	}
	message := <-t.readIn
	return message, nil
}

func (t *beaconTower) Send(message *fireInfo) error {
	if t.isClose {
		return ErrorClose
	}
	t.sendOut <- message
	return nil
}

func (t *beaconTower) Close() {
	t.ws.Close()
	t.mutex.Lock()
	defer t.mutex.Unlock()
	if !t.isClose {
		t.isClose = true
		close(t.closeSwitch)
	}
}

func (t *beaconTower) sendLoop() {
	for {
		select {
		case message := <-t.sendOut:
			if err := t.ws.WriteMessage(message.messageType, message.data); err != nil {
				goto collapse
			}
		case <-t.closeSwitch:
			return
		}
	}
collapse:
	t.Close()
}

func (t *beaconTower) readLoop() {
	for {
		messageType, data, err := t.ws.ReadMessage() // 内部声明可以及时释放内存
		if err != nil {
			goto collapse // 出现问题烽火台直接坍塌
		}
		message := &fireInfo{
			messageType: messageType,
			data:        data,
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

func (t *beaconTower) procLoop() {
	// 发送心跳包
	go func() {
		heartTicker := time.NewTicker(time.Duration(BTConfig.heartbeat) * time.Second)
		for {
			select {
			case <-t.closeSwitch:
				heartTicker.Stop()
				return
			case <-heartTicker.C:
				message := &fireInfo{
					messageType: websocket.TextMessage,
					data:        []byte(BTConfig.heartbeatContent),
				}
				if err := t.Send(message); err != nil {
					fmt.Println("heartbeat send failed:", err)
					t.Close()
					return
				}
			}
		}
	}()

	for {
		message, err := t.Read()
		if err != nil {
			fmt.Println("read message failed:", err)
			break
		}
		// TODO 消息事件 callback通知业务方
		fmt.Println("new message:", string(message.data))
	}
}
