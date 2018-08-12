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

var topicRelevance sync.Map

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
		num = int64(value.(*list.List).Len())
	}
	return &pb.GetConnectNumResponse{Number: num}, nil
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

func tcpHandler(conn net.Conn) {
	for {
		var a = make([]byte, 1024)
		l, err := conn.Read(a)
		if err != nil {
			conn.Close()
			return
		}
		fmt.Println("new message:", string(a))
		message := new(socket.TopicEvent)
		err = json.Unmarshal(a[:l], &message)
		if err != nil {
			fmt.Printf("topic message format error:%v\n", err)
			continue
		}
		for _, topic := range message.Topic {
			if message.Type == socket.SubKey {
				var store *list.List
				value, ok := topicRelevance.Load(topic)
				if !ok {
					// topic map 里面维护一个链表
					store = list.New()
					store.PushBack(&topicRelevanceItem{
						ip:   conn.RemoteAddr().String(),
						num:  1,
						conn: conn,
					})
					topicRelevance.Store(topic, store)
				} else {
					ip := conn.RemoteAddr().String()
					store = value.(*list.List)
					for e := store.Front(); e != nil; e = e.Next() {
						if e.Value.(*topicRelevanceItem).ip == ip {
							e.Value.(*topicRelevanceItem).num++
						}
					}
				}
				b, err := json.Marshal(&socket.PushMessage{
					Topic: topic,
					Type:  message.Type,
				})
				for e := store.Front(); e != nil; e = e.Next() {
					fmt.Println("pushd", e.Value.(*topicRelevanceItem).ip, string(b))
					_, err = e.Value.(*topicRelevanceItem).conn.Write(b)
					if err != nil {
						// 直接操作table.Remove 可以改变map中list的值
						e.Value.(*topicRelevanceItem).conn.Close()
						store.Remove(e)
					}
				}
				fmt.Println("topicRelevance:")
				topicRelevance.Range(func(key, value interface{}) bool {
					fmt.Println(key, value)
					return true
				})
			} else if message.Type == socket.UnSubKey { // 取消订阅事件
				value, ok := topicRelevance.Load(topic)
				if !ok {
					// topic 没有存在订阅列表中直接过滤
					continue
				} else {
					ip := conn.RemoteAddr().String()
					store := value.(*list.List)
					for e := store.Front(); e != nil; e = e.Next() {
						if e.Value.(*topicRelevanceItem).ip == ip {
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
					b, err := json.Marshal(&socket.PushMessage{
						Topic: topic,
						Type:  message.Type,
					})
					for e := store.Front(); e != nil; e = e.Next() {
						fmt.Println("pushd", e.Value.(*topicRelevanceItem).ip, string(b))
						_, err = e.Value.(*topicRelevanceItem).conn.Write(b)
						if err != nil {
							// 直接操作table.Remove 可以改变map中list的值
							e.Value.(*topicRelevanceItem).conn.Close()
							store.Remove(e)
						}
					}
				}

				fmt.Println("topicRelevance:")
				topicRelevance.Range(func(key, value interface{}) bool {
					fmt.Println(key, value)
					return true
				})
			} else if message.Type == socket.PublishKey {
				value, ok := topicRelevance.Load(topic)
				if !ok {
					// topic 没有存在订阅列表中直接过滤
					continue
				} else {
					b, err := json.Marshal(&socket.PushMessage{
						Topic: topic,
						Data:  message.DATA,
						Type:  message.Type,
					})
					table := value.(*list.List)
					for e := table.Front(); e != nil; e = e.Next() {
						fmt.Println("pushd", e.Value.(*topicRelevanceItem).ip, string(b))
						_, err = e.Value.(*topicRelevanceItem).conn.Write(b)
						if err != nil {
							// 直接操作table.Remove 可以改变map中list的值
							e.Value.(*topicRelevanceItem).conn.Close()
							table.Remove(e)
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
			conn.Close()
			return
		}
	}
}
