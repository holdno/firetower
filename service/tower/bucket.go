package tower

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/gorilla/websocket"
	"github.com/holdno/firetower/config"
	"github.com/holdno/firetower/pkg/nats"
	"github.com/holdno/firetower/protocol"
	"github.com/holdno/firetower/store"
	"github.com/holdno/firetower/store/redis"
	"github.com/holdno/firetower/store/single"
	"github.com/holdno/firetower/utils"

	cmap "github.com/orcaman/concurrent-map/v2"
)

type Manager[T any] interface {
	protocol.Pusher[T]
	BuildTower(ws *websocket.Conn, clientId string) (tower *FireTower[T], err error)
	BuildServerSideTower(clientId string) ServerSideTower[T]
	NewFire(source protocol.FireSource, tower PusherInfo) *protocol.FireInfo[T]
	GetTopics() (map[string]uint64, error)
	ClusterID() int64
	Store() stores
	Logger() protocol.Logger
}

// TowerManager 包含中心处理队列和多个bucket
// bucket的作用是将一个实例的连接均匀的分布在多个bucket中来达到并发推送的目的
type TowerManager[T any] struct {
	cfg         config.FireTowerConfig
	bucket      []*Bucket[T]
	centralChan chan *protocol.FireInfo[T] // 中心处理队列
	ip          string
	clusterID   int64
	timeout     time.Duration

	stores       stores
	logger       protocol.Logger
	topicCounter chan counterMsg
	connCounter  chan counterMsg

	coder protocol.Coder[T]
	protocol.Pusher[T]

	isClose   bool
	closeChan chan struct{}

	brazier protocol.Brazier[T]

	onTopicCountChangedHandler func(Topic string)
	onConnCountChangedHandler  func()
}

func (t *TowerManager[T]) SetTopicCountChangedHandler(f func(string)) {
	t.onTopicCountChangedHandler = f
}

func (t *TowerManager[T]) SetConnCountChangedHandler(f func()) {
	t.onConnCountChangedHandler = f
}

type counterMsg struct {
	Key string
	Num int64
}

type stores interface {
	ClusterConnStore() store.ClusterConnStore
	ClusterTopicStore() store.ClusterTopicStore
	ClusterStore() store.ClusterStore
}

// Bucket 的作用是将一个实例的连接均匀的分布在多个bucket中来达到并发推送的目的
type Bucket[T any] struct {
	tm  *TowerManager[T]
	mu  sync.RWMutex // 读写锁，可并发读不可并发读写
	id  int64
	len int64
	// topicRelevance map[string]map[string]*FireTower // topic -> websocket clientid -> websocket conn
	topicRelevance cmap.ConcurrentMap[string, cmap.ConcurrentMap[string, *FireTower[T]]]
	BuffChan       chan *protocol.FireInfo[T] // bucket的消息处理队列
	sendTimeout    time.Duration
}

type TowerOption[T any] func(t *TowerManager[T])

func BuildWithPusher[T any](pusher protocol.Pusher[T]) TowerOption[T] {
	return func(t *TowerManager[T]) {
		t.Pusher = pusher
	}
}

func BuildWithMessageDisposeTimeout[T any](timeout time.Duration) TowerOption[T] {
	return func(t *TowerManager[T]) {
		if timeout > 0 {
			t.timeout = timeout
		}
	}
}

func BuildWithCoder[T any](coder protocol.Coder[T]) TowerOption[T] {
	return func(t *TowerManager[T]) {
		t.coder = coder
	}
}

func BuildWithStore[T any](store stores) TowerOption[T] {
	return func(t *TowerManager[T]) {
		t.stores = store
	}
}

func BuildWithClusterID[T any](id int64) TowerOption[T] {
	return func(t *TowerManager[T]) {
		t.clusterID = id
	}
}

func BuildWithLogger[T any](logger protocol.Logger) TowerOption[T] {
	return func(t *TowerManager[T]) {
		t.logger = logger
	}
}

func BuildFoundation[T any](cfg config.FireTowerConfig, opts ...TowerOption[T]) (Manager[T], error) {
	tm := &TowerManager[T]{
		cfg:          cfg,
		bucket:       make([]*Bucket[T], cfg.Bucket.Num),
		centralChan:  make(chan *protocol.FireInfo[T], cfg.Bucket.CentralChanCount),
		topicCounter: make(chan counterMsg, 20000),
		connCounter:  make(chan counterMsg, 20000),
		closeChan:    make(chan struct{}),
		brazier:      newBrazier[T](),
		logger: slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})),
		timeout: time.Second,
	}

	for _, opt := range opts {
		opt(tm)
	}

	var err error
	if tm.ip, err = utils.GetIP(); err != nil {
		panic(err)
	}

	if tm.coder == nil {
		tm.coder = &protocol.DefaultCoder[T]{}
	}

	if tm.Pusher == nil {
		if cfg.ServiceMode == config.SingleMode {
			tm.Pusher = protocol.DefaultPusher(tm.brazier, tm.coder, tm.logger)
		} else {
			tm.Pusher = nats.MustSetupNatsPusher(cfg.Cluster.NatsOption, tm.coder, tm.logger, func() map[string]uint64 {
				m, err := tm.stores.ClusterTopicStore().Topics()
				if err != nil {
					tm.logger.Error("failed to get current node topics from nats", slog.Any("error", err))
					return map[string]uint64{}
				}
				return m
			})
		}
	}

	if tm.stores == nil {
		if cfg.ServiceMode == config.SingleMode {
			tm.stores, _ = single.Setup()
		} else {
			if tm.stores, err = redis.Setup(cfg.Cluster.RedisOption.Addr,
				cfg.Cluster.RedisOption.Password,
				cfg.Cluster.RedisOption.DB, tm.ip,
				cfg.Cluster.RedisOption.KeyPrefix); err != nil {
				panic(err)
			}
		}
	}

	if tm.clusterID == 0 {
		clusterID, err := tm.stores.ClusterStore().ClusterNumber()
		if err != nil {
			return nil, err
		}
		tm.clusterID = clusterID
	}
	utils.SetupIDWorker(tm.clusterID)

	for i := range tm.bucket {
		tm.bucket[i] = newBucket[T](tm, cfg.Bucket.BuffChanCount, cfg.Bucket.ConsumerNum)
	}

	go func() {
		var (
			connCounter      int64
			topicCounter     = make(map[string]int64)
			ticker           = time.NewTicker(time.Millisecond * 500)
			clusterHeartbeat = time.NewTicker(time.Second * 3)
		)

		reportConn := func(counter int64) {
			err := retry.Do(func() error {
				return tm.stores.ClusterConnStore().OneClientAtomicAddBy(tm.ip, counter)
			}, retry.Attempts(3), retry.LastErrorOnly(true))
			if err != nil {
				tm.logger.Error("failed to update the number of redis websocket connections", slog.Any("error", err))
				tm.connCounter <- counterMsg{
					Key: tm.ip,
					Num: counter,
				}
				return
			}
			if tm.onConnCountChangedHandler != nil {
				tm.onConnCountChangedHandler()
			}
		}

		reportTopicConn := func(topicCounter map[string]int64) {
			for t, n := range topicCounter {
				err := retry.Do(func() error {
					return tm.stores.ClusterTopicStore().TopicConnAtomicAddBy(t, n)
				}, retry.Attempts(3), retry.LastErrorOnly(true))
				if err != nil {
					tm.logger.Error("failed to update the number of connections for the topic in redis", slog.Any("error", err))
					tm.topicCounter <- counterMsg{
						Key: t,
						Num: n,
					}
					return
				}
				if tm.onTopicCountChangedHandler != nil {
					tm.onTopicCountChangedHandler(t)
				}
			}
		}
		for {
			select {
			case msg := <-tm.connCounter:
				connCounter += msg.Num
			case msg := <-tm.topicCounter:
				topicCounter[msg.Key] += msg.Num
			case <-ticker.C:
				if connCounter > 0 {
					go reportConn(connCounter)
					connCounter = 0
					clusterHeartbeat.Reset(time.Second * 3)
				}
				if len(topicCounter) > 0 {
					go reportTopicConn(topicCounter)
					topicCounter = make(map[string]int64)
				}
			case <-clusterHeartbeat.C:
				reportConn(0)
			case <-tm.closeChan:
				return
			}
		}
	}()

	// 执行中心处理器 将所有推送消息推送到bucketNum个bucket中
	go func() {
		for {
			select {
			case fire := <-tm.Receive():
				for _, b := range tm.bucket {
					b.BuffChan <- fire
				}
			case <-tm.closeChan:
				return
			}
		}
	}()

	return tm, nil
}

func (t *TowerManager[T]) Logger() protocol.Logger {
	return t.logger
}

func (t *TowerManager[T]) Store() stores {
	return t.stores
}

func (t *TowerManager[T]) GetTopics() (map[string]uint64, error) {
	return t.stores.ClusterTopicStore().ClusterTopics()
}

func (t *TowerManager[T]) ClusterID() int64 {
	if t == nil {
		panic("firetower cluster not setup")
	}
	return t.clusterID
}

func newBucket[T any](tm *TowerManager[T], buff int64, consumerNum int) *Bucket[T] {
	b := &Bucket[T]{
		tm:             tm,
		id:             getNewBucketId(),
		len:            0,
		topicRelevance: cmap.New[cmap.ConcurrentMap[string, *FireTower[T]]](),
		BuffChan:       make(chan *protocol.FireInfo[T], buff),
		sendTimeout:    tm.timeout,
	}

	if consumerNum == 0 {
		consumerNum = 1
	}
	// 每个bucket启动ConsumerNum个消费者(并发处理)
	for i := 0; i < consumerNum; i++ {
		go b.consumer()
	}
	return b
}

var (
	bucketId int64
	connId   uint64
)

func getNewBucketId() int64 {
	atomic.AddInt64(&bucketId, 1)
	return bucketId
}

func getConnId() uint64 {
	atomic.AddUint64(&connId, 1)
	return connId
}

// GetBucket 获取一个可以分配当前连接的bucket
func (t *TowerManager[T]) GetBucket(bt *FireTower[T]) (bucket *Bucket[T]) {
	bucket = t.bucket[bt.connID%uint64(len(t.bucket))]
	return
}

// 来自publish的消息
func (b *Bucket[T]) consumer() {
	for {
		select {
		case fire := <-b.BuffChan:
			switch fire.Message.Type {
			case protocol.OfflineTopicByUserIdOperation:
				// 需要退订的topic和user_id
				// todo use api
				b.unSubscribeByUserId(fire)
			case protocol.OfflineTopicOperation:
				// todo use api
				b.unSubscribeAll(fire)
			case protocol.OfflineUserOperation:
				// todo use api
				b.offlineUsers(fire)
			default:
				b.push(fire)
			}
		}
	}
}

// AddSubscribe 添加当前实例中的topic->conn的订阅关系
func (b *Bucket[T]) AddSubscribe(topic string, bt *FireTower[T]) {
	if m, ok := b.topicRelevance.Get(topic); ok {
		m.Set(bt.ClientID(), bt)
	} else {
		inner := cmap.New[*FireTower[T]]()
		inner.Set(bt.ClientID(), bt)
		b.topicRelevance.Set(topic, inner)
	}
	b.tm.topicCounter <- counterMsg{
		Key: topic,
		Num: 1,
	}
}

// DelSubscribe 删除当前实例中的topic->conn的订阅关系
func (b *Bucket[T]) DelSubscribe(topic string, bt *FireTower[T]) {
	if inner, ok := b.topicRelevance.Get(topic); ok {
		inner.Remove(bt.clientID)
		if inner.IsEmpty() {
			b.topicRelevance.Remove(topic)
		}
	}

	b.tm.topicCounter <- counterMsg{
		Key: topic,
		Num: -1,
	}
}

// Push 桶内进行遍历push
// 每个bucket有一个Push方法
// 在推送时每个bucket同时调用Push方法 来达到并发推送
// 该方法主要通过遍历桶中的topic->conn订阅关系来进行websocket写入
func (b *Bucket[T]) push(message *protocol.FireInfo[T]) error {
	if m, ok := b.topicRelevance.Get(message.Message.Topic); ok {
		for _, v := range m.Items() {
			if v.isClose {
				continue
			}

			if v.receivedHandler != nil && !v.receivedHandler(message) {
				continue
			}

			if v.ws != nil {
				func() {
					ctx, cancel := context.WithTimeout(context.Background(), b.sendTimeout)
					defer cancel()
					select {
					case v.sendOut <- message.Message.Json():
					case <-ctx.Done():
						if v.sendTimeoutHandler != nil {
							v.sendTimeoutHandler(message)
						}
					}
				}()
			}
		}
	}
	return nil
}

// UnSubscribeByUserId 服务端指定某个用户退订某个topic
func (b *Bucket[T]) unSubscribeByUserId(fire *protocol.FireInfo[T]) error {
	if m, ok := b.topicRelevance.Get(fire.Message.Topic); ok {
		userId, ok := fire.Context.ExtMeta[protocol.SYSTEM_CMD_REMOVE_USER]
		if ok {
			for _, v := range m.Items() {
				if v.UserID() == userId {
					v.unbindTopic([]string{fire.Message.Topic})
					if v.unSubscribeHandler != nil {
						v.unSubscribeHandler(fire.Context, []string{fire.Message.Topic})
					}
					return nil
				}
			}
			return nil
		}
	}

	return ErrorTopicEmpty
}

// UnSubscribeAll 移除所有该topic的订阅关系
func (b *Bucket[T]) unSubscribeAll(fire *protocol.FireInfo[T]) error {
	if m, ok := b.topicRelevance.Get(fire.Message.Topic); ok {
		for _, v := range m.Items() {
			v.unbindTopic([]string{fire.Message.Topic})
			// 移除所有人的应该不需要执行回调方法
			if v.onSystemRemove != nil {
				v.onSystemRemove(fire.Message.Topic)
			}
		}
		return nil
	}
	return ErrorTopicEmpty
}

func (b *Bucket[T]) offlineUsers(fire *protocol.FireInfo[T]) error {
	if m, ok := b.topicRelevance.Get(fire.Message.Topic); ok {
		userId, ok := fire.Context.ExtMeta[protocol.SYSTEM_CMD_REMOVE_USER]
		if ok {
			for _, v := range m.Items() {
				if v.UserID() == userId {
					v.Close()
					return nil
				}
			}
		}
	}
	return ErrorTopicEmpty
}
