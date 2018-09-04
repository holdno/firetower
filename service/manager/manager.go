package manager

import (
	"container/list"
	"context"
	"fmt"
	pb "github.com/holdno/firetower/grpc/manager"
	"github.com/holdno/firetower/socket"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"net"
	"sync"
	"time"
)

type Manager struct {
	logger func(t, info string)
}

var (
	topicRelevance sync.Map
	ConnIndexTable sync.Map
	prefix         = "[firetower manager]"
)

type topicRelevanceItem struct {
	ip   string
	num  int64
	conn net.Conn
}

type topicGrpcService struct {
	mu sync.RWMutex

	logInfo  func(info string)
	logError func(err string)
}

// Publish
func (t *topicGrpcService) Publish(ctx context.Context, request *pb.PublishRequest) (*pb.PublishResponse, error) {
	t.logInfo(fmt.Sprintf("new message:", string(request.Data)))

	value, ok := topicRelevance.Load(request.Topic)
	if !ok {
		// topic 没有存在订阅列表中直接过滤
		return &pb.PublishResponse{Ok: false}, errors.New("topic not exist")
	} else {

		table := value.(*list.List)
		t.mu.Lock()
		for e := table.Front(); e != nil; e = e.Next() {

			c, ok := ConnIndexTable.Load(e.Value.(*topicRelevanceItem).ip)
			if ok {
				b, err := socket.Enpack(socket.PublishKey, request.Topic, request.Data)
				if err != nil {

				}
				_, err = c.(*connectBucket).conn.Write(b)
				if err != nil {
					c.(*connectBucket).close()
				}
			}
		}
		t.mu.Unlock()
	}

	return &pb.PublishResponse{Ok: true}, nil
}

// 获取topic订阅数
func (t *topicGrpcService) GetConnectNum(ctx context.Context, request *pb.GetConnectNumRequest) (*pb.GetConnectNumResponse, error) {
	value, ok := topicRelevance.Load(request.Topic)
	var num int64
	if ok {
		l, _ := value.(*list.List)
		for e := l.Front(); e != nil; e = e.Next() {
			num += e.Value.(*topicRelevanceItem).num
		}
	}
	return &pb.GetConnectNumResponse{Number: num}, nil
}

// topic 订阅
func (t *topicGrpcService) SubscribeTopic(ctx context.Context, request *pb.SubscribeTopicRequest) (*pb.SubscribeTopicResponse, error) {
	for _, topic := range request.Topic {
		var store *list.List
		value, ok := topicRelevance.Load(topic)

		if !ok {
			// topic map 里面维护一个链表
			store = list.New()
			store.PushBack(&topicRelevanceItem{
				ip:  request.Ip,
				num: 1,
			})
			topicRelevance.Store(topic, store)
		} else {
			store = value.(*list.List)
			for e := store.Front(); e != nil; e = e.Next() {
				if e.Value.(*topicRelevanceItem).ip == request.Ip {
					e.Value.(*topicRelevanceItem).num++
				}
			}
		}
	}
	return &pb.SubscribeTopicResponse{}, nil
}

// topic 取消订阅
func (t *topicGrpcService) UnSubscribeTopic(ctx context.Context, request *pb.UnSubscribeTopicRequest) (*pb.UnSubscribeTopicResponse, error) {
	for _, topic := range request.Topic {
		value, ok := topicRelevance.Load(topic)

		if !ok {
			// topic 没有存在订阅列表中直接过滤
			continue
		} else {
			store := value.(*list.List)
			for e := store.Front(); e != nil; e = e.Next() {
				if e.Value.(*topicRelevanceItem).ip == request.Ip {
					if e.Value.(*topicRelevanceItem).num-1 == 0 {
						store.Remove(e)
						if store.Len() == 0 {
							topicRelevance.Delete(topic)
						}
					} else {
						// 这里修改是直接修改map内部值
						e.Value.(*topicRelevanceItem).num--
					}
					break
				}
			}
		}
	}
	return &pb.UnSubscribeTopicResponse{}, nil
}

func (m *Manager) StartGrpcService(port string) {
	lis, err := net.Listen("tcp", port)
	if err != nil {
		m.logError(fmt.Sprintf("grpc service listen error: %v", err))
	}
	s := grpc.NewServer()
	pb.RegisterTopicServiceServer(s, &topicGrpcService{
		logInfo:  m.logInfo,
		logError: m.logError,
	})
	s.Serve(lis)
}

type connectBucket struct {
	overflow   []byte
	packetChan chan *socket.PushMessage
	conn       net.Conn
	isClose    bool
	closeChan  chan struct{}
	mu         sync.Mutex

	logInfo  func(info string)
	logError func(err string)
}

func (m *Manager) StartSocketService(addr string) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		m.logError(fmt.Sprintf("tcp service listen error: %v", err))
		return
	}
	for {
		conn, err := lis.Accept()
		if err != nil {
			m.logError(fmt.Sprintf("tcp service accept error: %v", err))
			continue
		}
		bucket := &connectBucket{
			overflow:   make([]byte, 0),
			packetChan: make(chan *socket.PushMessage, 1024),
			conn:       conn,
			isClose:    false,
			closeChan:  make(chan struct{}),

			logInfo:  m.logInfo,
			logError: m.logError,
		}
		bucket.relation()     // 建立连接关系
		go bucket.sendLoop()  // 发包
		go bucket.handler()   // 接收字节流并解包
		go bucket.heartbeat() // 心跳
	}
}

func (m *Manager) logInfo(info string) {
	if m.logger != nil {
		m.logger("INFO", info)
	} else {
		fmt.Printf("%s %s %s %s\n", prefix, time.Now().Format("2006-01-02 15:04:05"), "INFO", info)
	}
}

func (m *Manager) logError(info string) {
	if m.logger != nil {
		m.logger("Error", info)
	} else {
		fmt.Printf("%s %s %s %s\n", prefix, time.Now().Format("2006-01-02 15:04:05"), "ERROR", info)
	}
}

func (m *Manager) ReplaceLog(fn func(t, info string)) {
	m.logger = fn
}

func (c *connectBucket) relation() {
	// 维护一个IP->连接关系的索引map
	_, ok := ConnIndexTable.Load(c.conn.RemoteAddr().String())
	if !ok {
		c.logInfo(fmt.Sprintf("新连接: %s", c.conn.RemoteAddr().String()))
		ConnIndexTable.Store(c.conn.RemoteAddr().String(), c)
	}
}

func (c *connectBucket) delRelation() {
	topicRelevance.Range(func(key, value interface{}) bool {
		store, _ := value.(*list.List)
		for e := store.Front(); e != nil; e = e.Next() {
			if e.Value.(*topicRelevanceItem).ip == c.conn.RemoteAddr().String() {
				store.Remove(e)
			}
		}
		if store.Len() == 0 {
			topicRelevance.Delete(key)
		}
		return true
	})
}

func (c *connectBucket) close() {
	c.mu.Lock()
	if !c.isClose {
		c.isClose = true
		close(c.closeChan)
		c.conn.Close()
		c.delRelation() // 删除topic绑定关系
	}
	c.mu.Unlock()
}

func (c *connectBucket) handler() {
	for {
		var buffer = make([]byte, 1024*16)
		l, err := c.conn.Read(buffer)
		if err != nil {
			c.close()
			return
		}
		c.overflow, err = socket.Depack(append(c.overflow, buffer[:l]...), c.packetChan)
		if err != nil {
			c.logError(err.Error())
		}
	}
}

func (c *connectBucket) sendLoop() {
	for {
		select {
		case message := <-c.packetChan:
			if message.Type == socket.PublishKey {
				value, ok := topicRelevance.Load(message.Topic)
				if !ok {
					// topic 没有存在订阅列表中直接过滤
					continue
				} else {
					table := value.(*list.List)
					for e := table.Front(); e != nil; e = e.Next() {
						bucket, ok := ConnIndexTable.Load(e.Value.(*topicRelevanceItem).ip)
						if ok {
							bytes, err := socket.Enpack(message.Type, message.Topic, message.Data)
							if err != nil {
								c.logError(fmt.Sprintf("protocol 封包时错误，%v", err))
							}
							_, err = bucket.(*connectBucket).conn.Write(bytes)
							if err != nil {
								// 直接操作table.Remove 可以改变map中list的值
								bucket.(*connectBucket).close()
								return
							}
						}
					}
				}
			}
		case <-c.closeChan:
			return
		}
	}
}

func (c *connectBucket) heartbeat() {
	t := time.NewTicker(1 * time.Minute)
	for {
		<-t.C
		b, _ := socket.Enpack("heartbeat", "*", []byte("heartbeat"))
		_, err := c.conn.Write(b)
		if err != nil {
			c.close()
			return
		}
	}
}
