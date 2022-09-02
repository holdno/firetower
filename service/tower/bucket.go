package tower

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/holdno/firetower/config"
	"github.com/holdno/firetower/pkg/nats"
	"github.com/holdno/firetower/protocol"
	"github.com/holdno/firetower/store"
	"github.com/holdno/firetower/store/redis"
	"github.com/holdno/firetower/store/single"
	"github.com/holdno/firetower/utils"

	"github.com/holdno/rego"
)

var (
	// tm 是一个实例的管理中心
	tm       *TowerManager
	firePool *sync.Pool
)

func init() {
	firePool = &sync.Pool{
		New: func() interface{} {
			return &protocol.FireInfo{
				Context: protocol.FireLife{},
				Message: protocol.TopicMessage{},
			}
		},
	}
}

// TowerManager 包含中心处理队列和多个bucket
// bucket的作用是将一个实例的连接均匀的分布在多个bucket中来达到并发推送的目的
type TowerManager struct {
	cfg         config.FireTowerConfig
	bucket      []*Bucket
	centralChan chan *protocol.FireInfo // 中心处理队列
	ip          string
	clusterID   int64

	stores stores

	topicCounter chan counterMsg
	connCounter  chan counterMsg

	coder protocol.Coder
	protocol.Pusher

	isClose   bool
	closeChan chan struct{}

	brazier protocol.Brazier

	onTopicCountChangedHandler func(Topic string)
	onConnCountChangedHandler  func()
}

func (t *TowerManager) SetTopicCountChangedHandler(f func(string)) {
	t.onTopicCountChangedHandler = f
}

func (t *TowerManager) SetConnCountChangedHandler(f func()) {
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
type Bucket struct {
	mu             sync.RWMutex // 读写锁，可并发读不可并发读写
	id             int64
	len            int64
	topicRelevance map[string]map[string]*FireTower // topic -> websocket clientid -> websocket conn
	BuffChan       chan *protocol.FireInfo          // bucket的消息处理队列
}

type TowerOption func(t *TowerManager)

func BuildWithPusher(pusher protocol.Pusher) TowerOption {
	return func(t *TowerManager) {
		t.Pusher = pusher
	}
}

func BuildWithCoder(coder protocol.Coder) TowerOption {
	return func(t *TowerManager) {
		t.coder = coder
	}
}

func BuildWithStore(store stores) TowerOption {
	return func(t *TowerManager) {
		t.stores = store
	}
}

func BuildFoundation(cfg config.FireTowerConfig, opts ...TowerOption) error {
	tm = &TowerManager{
		cfg:          cfg,
		bucket:       make([]*Bucket, cfg.Bucket.Num),
		centralChan:  make(chan *protocol.FireInfo, cfg.Bucket.CentralChanCount),
		topicCounter: make(chan counterMsg, 200000),
		connCounter:  make(chan counterMsg, 200000),
		closeChan:    make(chan struct{}),
		brazier:      &brazier{},
	}

	for _, opt := range opts {
		opt(tm)
	}

	var err error
	if tm.ip, err = utils.GetIP(); err != nil {
		panic(err)
	}

	if tm.coder == nil {
		tm.coder = &protocol.DefaultCoder{}
	}

	if tm.Pusher == nil {
		if cfg.ServiceMode == config.SingleMode {
			tm.Pusher = protocol.DefaultPusher(tm.brazier, tm.coder)
		} else {
			tm.Pusher = nats.MustSetupNatsPusher(cfg.Cluster.NatsOption, tm.brazier, tm.coder, tm.GetTopics)
		}
	}

	if tm.stores == nil {
		if cfg.ServiceMode == config.SingleMode {
			tm.stores, _ = single.Setup()
		} else {
			if tm.stores, err = redis.Setup(cfg.Cluster.RedisOption.Addr,
				cfg.Cluster.RedisOption.Password,
				cfg.Cluster.RedisOption.DB, tm.ip); err != nil {
				panic(err)
			}
		}
	}

	clusterID, err := tm.stores.ClusterStore().ClusterNumber()
	if err != nil {
		return err
	}
	tm.clusterID = clusterID
	utils.SetupIDWorker(tm.clusterID)

	for i := range tm.bucket {
		tm.bucket[i] = newBucket(cfg.Bucket.BuffChanCount, cfg.Bucket.ConsumerNum)
	}

	go func() {
		var (
			connCounter  int64
			topicCounter = make(map[string]int64)
			ticker       = time.NewTicker(time.Millisecond * 500)
		)

		reportConn := func(counter int64) {
			err := rego.Retry(func() error {
				return tm.stores.ClusterConnStore().OneClientAtomicAddBy(tm.ip, counter)
			}, rego.WithLatestError(), rego.WithPeriod(time.Second), rego.WithBackoffFector(1.5), rego.WithTimes(16))
			if err != nil {
				// todo log & warning
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
				err := rego.Retry(func() error {
					return tm.stores.ClusterTopicStore().TopicConnAtomicAddBy(t, n)
				}, rego.WithLatestError(), rego.WithPeriod(time.Second), rego.WithBackoffFector(1.5), rego.WithTimes(10))
				if err != nil {
					// todo log & warning
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
				}
				if len(topicCounter) > 0 {
					go reportTopicConn(topicCounter)
					topicCounter = make(map[string]int64)
				}
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
				for i := range tm.bucket {
					tm.bucket[i].BuffChan <- fire
				}
				fire.Info("Sended")
			case <-tm.closeChan:
				return
			}
		}
	}()

	return nil
}

func (t *TowerManager) GetTopics() map[string]struct{} {
	topics := make(map[string]struct{})
	for _, v := range t.bucket {
		for k := range v.topicRelevance {
			topics[k] = struct{}{}
		}
	}
	return topics
}

func ClusterID() int64 {
	if tm == nil {
		panic("firetower cluster not setup")
	}
	return tm.clusterID
}

func newBucket(buff int64, consumerNum int) *Bucket {
	b := &Bucket{
		id:             getNewBucketId(),
		len:            0,
		topicRelevance: make(map[string]map[string]*FireTower),
		BuffChan:       make(chan *protocol.FireInfo, buff),
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
func (t *TowerManager) GetBucket(bt *FireTower) (bucket *Bucket) {
	bucket = t.bucket[bt.connID%uint64(len(t.bucket))]
	return
}

// 来自publish的消息
func (b *Bucket) consumer() {
	for {
		select {
		case fire := <-b.BuffChan:
			switch fire.Message.Type {
			case protocol.PublishKey:
				b.push(fire)
			case protocol.OfflineTopicByUserIdKey:
				// 需要退订的topic和user_id
				b.unSubscribeByUserId(fire)
			case protocol.OfflineTopicKey:
				b.unSubscribeAll(fire)
			case protocol.OfflineUserKey:
				b.offlineUsers(fire)
			}
		}
	}
}

// AddSubscribe 添加当前实例中的topic->conn的订阅关系
func (b *Bucket) AddSubscribe(topic string, bt *FireTower) {
	b.mu.Lock()
	if m, ok := b.topicRelevance[topic]; ok {
		m[bt.ClientID()] = bt
	} else {
		b.topicRelevance[topic] = make(map[string]*FireTower)
		b.topicRelevance[topic][bt.ClientID()] = bt
	}
	b.mu.Unlock()
	tm.topicCounter <- counterMsg{
		Key: topic,
		Num: 1,
	}
}

// DelSubscribe 删除当前实例中的topic->conn的订阅关系
func (b *Bucket) DelSubscribe(topic string, bt *FireTower) {
	b.mu.Lock()
	if m, ok := b.topicRelevance[topic]; ok {
		delete(m, bt.ClientID())
		if len(m) == 0 {
			delete(b.topicRelevance, topic)
		}
	}
	b.mu.Unlock()
	tm.topicCounter <- counterMsg{
		Key: topic,
		Num: -1,
	}
}

// Push 桶内进行遍历push
// 每个bucket有一个Push方法
// 在推送时每个bucket同时调用Push方法 来达到并发推送
// 该方法主要通过遍历桶中的topic->conn订阅关系来进行websocket写入
func (b *Bucket) push(message *protocol.FireInfo) error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if m, ok := b.topicRelevance[message.Message.Topic]; ok {
		for _, v := range m {
			v.Send(message)
		}
		return nil
	}
	return ErrorTopicEmpty
}

// UnSubscribeByUserId 服务端指定某个用户退订某个topic
func (b *Bucket) unSubscribeByUserId(fire *protocol.FireInfo) error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if m, ok := b.topicRelevance[fire.Message.Topic]; ok {
		userId := string(fire.Message.Data)
		for _, v := range m {
			if v.UserID() == userId {
				_, err := v.unbindTopic([]string{fire.Message.Topic})
				fire := NewFire(protocol.SourceSystem, v)
				defer tm.brazier.Extinguished(fire)
				if v.unSubscribeHandler != nil {
					v.unSubscribeHandler(fire.Context, []string{fire.Message.Topic})
				}
				if err != nil {
					return err
				}
				return nil
			}
		}
	}
	return ErrorTopicEmpty
}

// UnSubscribeAll 移除所有该topic的订阅关系
func (b *Bucket) unSubscribeAll(fire *protocol.FireInfo) error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if m, ok := b.topicRelevance[fire.Message.Topic]; ok {
		for _, v := range m {
			_, err := v.unbindTopic([]string{fire.Message.Topic})
			// 移除所有人的应该不需要执行回调方法
			if v.onSystemRemove != nil {
				v.onSystemRemove(fire.Message.Topic)
			}
			if err != nil {
				return err
			}
		}
	}
	return ErrorTopicEmpty
}

func (b *Bucket) offlineUsers(fire *protocol.FireInfo) error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if m, ok := b.topicRelevance[fire.Message.Topic]; ok {
		userId := string(fire.Message.Data)
		for _, v := range m {
			if v.UserID() == userId {
				v.Close()
				return nil
			}
		}
	}
	return ErrorTopicEmpty
}