package main

import (
	"container/list"
	"context"
	"encoding/json"
	"fmt"
	pb "github.com/holdno/beacon/grpc/topicmanage"
	"github.com/holdno/beacon/socket"
	"github.com/pelletier/go-toml"
	"google.golang.org/grpc"
	"net"
	"sync"
	"time"
)

var (
	topicRelevance sync.Map
	ConnIndexTable sync.Map
)

type topicRelevanceItem struct {
	ip   string
	num  int64
	conn net.Conn
}

var ConfigTree *toml.Tree

func init() {
	var (
		err error
	)
	if ConfigTree, err = toml.LoadFile("./topicmanage.toml"); err != nil {
		fmt.Println("config load failed:", err)
	}
}

func main() {
	go grpcService()
	tcpConnect()
}

type topicGrpcService struct{}

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

func grpcService() {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", ConfigTree.Get("grpc.port").(int64)))
	if err != nil {
		fmt.Println("grpc service listen error:", err)
	}
	s := grpc.NewServer()
	pb.RegisterTopicServiceServer(s, &topicGrpcService{})
	s.Serve(lis)
}

func tcpConnect() {
	lis, err := net.Listen("tcp", "0.0.0.0:6666")
	if err != nil {
		fmt.Println("tcp service listen error:", err)
		return
	}
	fmt.Println("topic service start: 0.0.0.0:6666")
	for {
		conn, err := lis.Accept()
		if err != nil {
			fmt.Println("tcp service accept error:", err)
			continue
		}
		fmt.Println("new socket connect")
		go tcpHandler(conn)
		go heartbeat(conn)
	}
}

func connOffLine(conn net.Conn) {
	conn.Close()
	topicRelevance.Range(func(key, value interface{}) bool {
		store, _ := value.(*list.List)
		for e := store.Front(); e != nil; e = e.Next() {
			if e.Value.(*topicRelevanceItem).ip == conn.RemoteAddr().String() {
				store.Remove(e)
			}
		}
		if store.Len() == 0 {
			topicRelevance.Delete(key)
		}
		return true
	})
}

func tcpHandler(conn net.Conn) {
	for {
		var a = make([]byte, 1024)
		l, err := conn.Read(a)
		if err != nil {
			connOffLine(conn)
			return
		}
		// 维护一个IP->连接关系的索引map
		_, ok := ConnIndexTable.Load(conn.RemoteAddr().String())
		if !ok {
			ConnIndexTable.Store(conn.RemoteAddr().String(), conn)
		}
		fmt.Println("new message:", string(a))
		message := new(socket.TopicEvent)
		err = json.Unmarshal(a[:l], &message)
		if err != nil {
			fmt.Printf("topic message format error:%v\n", err)
			continue
		}
		for _, topic := range message.Topic {
			if message.Type == socket.PublishKey {
				value, ok := topicRelevance.Load(topic)
				if !ok {
					// topic 没有存在订阅列表中直接过滤
					continue
				} else {
					b, _ := json.Marshal(&socket.PushMessage{
						Topic: topic,
						Data:  message.DATA,
						Type:  message.Type,
					})
					table := value.(*list.List)
					for e := table.Front(); e != nil; e = e.Next() {
						fmt.Println("pushd", e.Value.(*topicRelevanceItem).ip, string(b))

						conn, ok := ConnIndexTable.Load(e.Value.(*topicRelevanceItem).ip)
						if ok {
							_, err = conn.(net.Conn).Write(b)
							if err != nil {
								// 直接操作table.Remove 可以改变map中list的值
								connOffLine(conn.(net.Conn))
								continue
							}
						}
					}
				}
			}
		}
	}

}

func heartbeat(conn net.Conn) {
	t := time.NewTicker(60 * time.Second)
	for {
		<-t.C
		_, err := conn.Write([]byte("heartbeat"))
		if err != nil {
			connOffLine(conn)
			return
		}
	}
}
