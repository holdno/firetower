package manager

import (
	"container/list"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"net/http"

	"sort"

	"encoding/json"

	pb "github.com/OSMeteor/firetower/grpc/manager"
	"github.com/OSMeteor/firetower/socket"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

// Manager topic管理中心结构体
// 为了绑定一些专属的操作方法 所以创建了一个空的结构体
type Manager struct{}

var (
	// HttpAddress http服务监听端口配置
	HttpAddress    = ":8000"
	topicRelevance sync.Map
	// ConnIndexTable 连接关系索引表
	ConnIndexTable sync.Map

	// Logger 接管系统log t log类型 info log信息
	Logger func(types, info string) = log
	// LogLevel 日志打印级别
	LogLevel = "INFO"
	// DefaultWriter 正常日志的默认写入方式
	DefaultWriter io.Writer = os.Stdout
	// DefaultErrorWriter 错误日志的默认写入方式
	DefaultErrorWriter io.Writer = os.Stderr
)

type topicRelevanceItem struct {
	ip   string
	num  int64
	conn net.Conn
}

type topicGrpcService struct {
	mu sync.RWMutex
}

// Publish 推送的grpc接口
// request *pb.PublishRequest
// 接收 Topic 话题
//     Data  传输内容
//     MessageId gateway 来源的消息id
func (t *topicGrpcService) Publish(ctx context.Context, request *pb.PublishRequest) (*pb.PublishResponse, error) {
	Logger("INFO", fmt.Sprintf("new message: %s", string(request.Data)))

	value, ok := topicRelevance.Load(request.Topic)
	if !ok {
		// topic 没有存在订阅列表中直接过滤
		return &pb.PublishResponse{Ok: false}, errors.New("topic not exist")
	}

	table := value.(*list.List)
	t.mu.Lock()
	for e := table.Front(); e != nil; e = e.Next() {
		c, ok := ConnIndexTable.Load(e.Value.(*topicRelevanceItem).ip)
		if ok {
			b, err := socket.Enpack(socket.PublishKey, request.MessageId, request.Source, request.Topic, request.Data)
			if err != nil {

			}
			_, err = c.(*connectBucket).conn.Write(b)
			if err != nil {
				c.(*connectBucket).close()
			}
		}
	}
	t.mu.Unlock()

	return &pb.PublishResponse{Ok: true}, nil
}

// CheckTopicExist 检测topic是否已经存在订阅关系
func (t *topicGrpcService) CheckTopicExist(ctx context.Context, request *pb.CheckTopicExistRequest) (*pb.CheckTopicExistResponse, error) {
	_, ok := topicRelevance.Load(request.Topic)
	if !ok {
		// topic 没有存在订阅列表中直接过滤
		return &pb.CheckTopicExistResponse{Ok: false}, nil
	}
	return &pb.CheckTopicExistResponse{Ok: true}, nil
}

// GetConnectNum 获取topic订阅数的grpc接口
func (t *topicGrpcService) GetConnectNum(ctx context.Context, request *pb.GetConnectNumRequest) (*pb.GetConnectNumResponse, error) {
	value, ok := topicRelevance.Load(request.Topic)
	var num int64
	if ok {
		l, _ := value.(*list.List)
		num = getConnectNum(l)
	}

	return &pb.GetConnectNumResponse{Number: num}, nil
}

func getConnectNum(l *list.List) int64 {
	var num int64
	for e := l.Front(); e != nil; e = e.Next() {
		num += e.Value.(*topicRelevanceItem).num
	}
	return num
}

// SubscribeTopic 订阅topic的grpc接口
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

// UnSubscribeTopic 取消订阅topic的grpc接口
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

// StartGrpcService 启动grpc服务
// 包含 话题订阅 与 取消订阅 推送等
func (m *Manager) StartGrpcService(port string) {
	lis, err := net.Listen("tcp", port)
	if err != nil {
		Logger("ERROR", fmt.Sprintf("grpc service listen error: %v", err))
		panic(fmt.Sprintf("grpc service listen error: %v", err))
	}
	s := grpc.NewServer()
	pb.RegisterTopicServiceServer(s, &topicGrpcService{})
	s.Serve(lis)
}

type connectBucket struct {
	overflow   []byte
	packetChan chan *socket.SendMessage
	conn       net.Conn
	isClose    bool
	closeChan  chan struct{}
	mu         sync.Mutex
}

// StartSocketService 启动tcp服务
// 主要用来接收gateway的推送消息
func (m *Manager) StartSocketService(addr string) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		Logger("ERROR", fmt.Sprintf("tcp service listen error: %v", err))
		return
	}
	Logger("INFO", fmt.Sprintf("tcp service listening: %s", addr))
	for {
		conn, err := lis.Accept()
		if err != nil {
			Logger("ERROR", fmt.Sprintf("tcp service accept error: %v", err))
			continue
		}
		bucket := &connectBucket{
			overflow:   make([]byte, 0),
			packetChan: make(chan *socket.SendMessage, 1024),
			conn:       conn,
			isClose:    false,
			closeChan:  make(chan struct{}),
		}
		bucket.relation()     // 建立连接关系
		go bucket.sendLoop()  // 发包
		go bucket.handler()   // 接收字节流并解包
		go bucket.heartbeat() // 心跳
	}
}

func log(types, info string) {
	if types == "INFO" {
		if LogLevel != "INFO" {
			return
		}
		fmt.Fprintf(
			DefaultWriter,
			"[Firetower Manager] %s %s %s | LOGTIME %s | LOG %s\n",
			socket.Green, types, socket.Reset,
			time.Now().Format("2006-01-02 15:04:05"),
			info)
	} else {
		fmt.Fprintf(
			DefaultErrorWriter,
			"[Firetower Manager] %s %s %s | LOGTIME %s | LOG %s\n",
			socket.Red, types, socket.Reset,
			time.Now().Format("2006-01-02 15:04:05"),
			info)
	}
}

func (c *connectBucket) relation() {
	// 维护一个IP->连接关系的索引map
	_, ok := ConnIndexTable.Load(c.conn.RemoteAddr().String())
	if !ok {
		Logger("INFO", fmt.Sprintf("new connection: %s", c.conn.RemoteAddr().String()))
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
			Logger("ERROR", err.Error())
		}
	}
}

func (c *connectBucket) sendLoop() {
	for {
		select {
		case message := <-c.packetChan:

			value, ok := topicRelevance.Load(message.Topic)
			if !ok {
				// topic 没有存在订阅列表中直接过滤
				continue
			} else {
				table := value.(*list.List)
				for e := table.Front(); e != nil; e = e.Next() {
					bucket, ok := ConnIndexTable.Load(e.Value.(*topicRelevanceItem).ip)
					if ok {
						bytes, err := socket.Enpack(message.Type, message.Context.Id, message.Context.Source, message.Topic, message.Data)
						if err != nil {
							Logger("ERROR", fmt.Sprintf("protocol 封包时错误，%v", err))
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
			message.Info("topic manager sended")
			message.Recycling()
		case <-c.closeChan:
			return
		}
	}
}

func (c *connectBucket) heartbeat() {
	t := time.NewTicker(10 * time.Second)
	// 心跳包内容固定 所以只用封包一次 直接用封好的包发送就可以了
	// 服务器间心跳时间应该短一些，以便及时获取连接状态
	b, _ := socket.Enpack("heartbeat", "0", "system", "*", []byte("heartbeat"))
	for {
		<-t.C
		_, err := c.conn.Write(b)
		if err != nil {
			c.close()
			return
		}
	}
}

// web service
// Response webservice 返回数据结构体
type Response struct {
	Meta *Meta  `json:"meta"`
	Data string `json:"data"`
}

// Meta http response 状态标识
type Meta struct {
	Code  int    `json:"code"`
	Error string `json:"error"`
}

// TopicWebRes topic信息面板结构体
type TopicWebRes struct {
	Title      string `json:"title"`
	ConnectNum int64  `json:"connect_num"`
}

type topics []*TopicWebRes

func (c topics) Len() int {
	return len(c)
}
func (c topics) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}
func (c topics) Less(i, j int) bool {
	return c[i].ConnectNum > c[j].ConnectNum
}

// HttpDashboard  dashboard http 服务
func HttpDashboard() {
	// http.HandleFunc("/", Dashboard)
	http.HandleFunc("/topic", topicWebHandler)
	http.ListenAndServe(HttpAddress, nil)
}

// func Dashboard(w http.ResponseWriter, r *http.Request) {
//	 w.Header().Set("Content-Type", "text/html; charset=utf-8")
//	 w.WriteHeader(200)
//	 template.ParseFiles("")
// }

// TopicWebHandler 获取topic相关统计信息
func topicWebHandler(w http.ResponseWriter, r *http.Request) {
	var topicSlice topics
	topicRelevance.Range(func(key, value interface{}) bool {
		var t = new(TopicWebRes)
		t.Title = key.(string)
		value, ok := topicRelevance.Load(t.Title)
		var num int64
		if ok {
			l, _ := value.(*list.List)
			num = getConnectNum(l)
		}
		t.ConnectNum = num
		topicSlice = append(topicSlice, t)
		return true
	})

	sort.Sort(topicSlice)
	res, err := json.Marshal(topicSlice)
	response := new(Response)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err != nil {
		w.WriteHeader(500)
		meta := &Meta{
			Code:  5001,
			Error: err.Error(),
		}
		response.Meta = meta
	} else {
		meta := &Meta{
			Code:  0,
			Error: "",
		}
		response.Meta = meta
		response.Data = string(res)
	}
	bytes, _ := json.Marshal(response)
	w.Write(bytes)
}
