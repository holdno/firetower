package gateway

import (
	"sync"
	"sync/atomic"

	"github.com/OSMeteor/firetower/socket"
)

var (
	// TM 是一个实例的管理中心
	TM *TowerManager
)

// TowerManager 包含中心处理队列和多个bucket
// bucket的作用是将一个实例的连接均匀的分布在多个bucket中来达到并发推送的目的
type TowerManager struct {
	bucket      []*Bucket
	centralChan chan *socket.SendMessage // 中心处理队列
}

// Bucket 的作用是将一个实例的连接均匀的分布在多个bucket中来达到并发推送的目的
type Bucket struct {
	mu             sync.RWMutex // 读写锁，可并发读不可并发读写
	id             int64
	len            int64
	topicRelevance map[string]map[string]*FireTower // topic -> websocket clientid -> websocket conn
	BuffChan       chan *socket.SendMessage         // bucket的消息处理队列
}

func buildBuckets() {
	bucketNum := int(ConfigTree.Get("bucket.Num").(int64))
	TM = &TowerManager{
		bucket:      make([]*Bucket, bucketNum),
		centralChan: make(chan *socket.SendMessage, ConfigTree.Get("bucket.CentralChanCount").(int64)),
	}

	for i := 0; i < bucketNum; i++ {
		TM.bucket[i] = newBucket()
	}

	// 执行中心处理器 将所有推送消息推送到bucketNum个bucket中
	go func() {
		for {
			select {
			case message := <-TM.centralChan:
				for i := 0; i < bucketNum; i++ {
					TM.bucket[i].BuffChan <- message
				}
				message.Info("Sended")
				message.Recycling()
			}
		}
	}()
}

func newBucket() *Bucket {
	b := &Bucket{
		id:             getNewBucketId(),
		len:            0,
		topicRelevance: make(map[string]map[string]*FireTower),
		BuffChan:       make(chan *socket.SendMessage, ConfigTree.Get("bucket.BuffChanCount").(int64)),
	}

	ConsumerNum := int(ConfigTree.Get("bucket.ConsumerNum").(int64))
	if ConsumerNum == 0 {
		ConsumerNum = 1
	}
	// 每个bucket启动ConsumerNum个消费者(并发处理)
	for i := 0; i < ConsumerNum; i++ {
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
	bucket = t.bucket[bt.connId%uint64(len(t.bucket))]
	return
}

func (b *Bucket) consumer() {
	for {
		select {
		case message := <-b.BuffChan:
			switch message.Type {
			case socket.PublishKey:
				b.push(message)
			case socket.OfflineTopicByUserIdKey:
				// 需要退订的topic和user_id
				b.unSubscribeByUserId(message)
			case socket.OfflineTopicKey:
				b.unSubscribeAll(message)
			case socket.OfflineUserKey:
				b.offlineUsers(message)
			}
			if message.Type == "push" {

			}

		}
	}
}

// AddSubscribe 添加当前实例中的topic->conn的订阅关系
func (b *Bucket) AddSubscribe(topic string, bt *FireTower) {
	b.mu.Lock()
	if m, ok := b.topicRelevance[topic]; ok {
		m[bt.ClientId] = bt
	} else {
		b.topicRelevance[topic] = make(map[string]*FireTower)
		b.topicRelevance[topic][bt.ClientId] = bt
	}
	b.mu.Unlock()
}

// DelSubscribe 删除当前实例中的topic->conn的订阅关系
func (b *Bucket) DelSubscribe(topic string, bt *FireTower) {
	b.mu.Lock()
	if m, ok := b.topicRelevance[topic]; ok {
		delete(m, bt.ClientId)
		if len(m) == 0 {
			delete(b.topicRelevance, topic)
		}
	}
	b.mu.Unlock()
}

// Push 桶内进行遍历push
// 每个bucket有一个Push方法
// 在推送时每个bucket同时调用Push方法 来达到并发推送
// 该方法主要通过遍历桶中的topic->conn订阅关系来进行websocket写入
func (b *Bucket) push(message *socket.SendMessage) error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if m, ok := b.topicRelevance[message.Topic]; ok {
		for _, v := range m {
			v.Send(message)
		}
		return nil
	}
	return ErrorTopicEmpty
}

// UnSubscribeByUserId 服务端指定某个用户退订某个topic
func (b *Bucket) unSubscribeByUserId(message *socket.SendMessage) error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if m, ok := b.topicRelevance[message.Topic]; ok {
		userId := string(message.Data)
		for _, v := range m {
			if v.UserId == userId {
				_, err := v.unbindTopic([]string{message.Topic})
				v.ToSelf([]byte("{}"))
				if v.unSubscribeHandler != nil {
					v.unSubscribeHandler(nil, []string{message.Topic})
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
func (b *Bucket) unSubscribeAll(message *socket.SendMessage) error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if m, ok := b.topicRelevance[message.Topic]; ok {
		for _, v := range m {
			_, err := v.unbindTopic([]string{message.Topic})
			// 移除所有人的应该不需要执行回调方法
			if v.onSystemRemove != nil {
				v.onSystemRemove(message.Topic)
			}
			if err != nil {
				return err
			}
		}
	}
	return ErrorTopicEmpty
}

func (b *Bucket) offlineUsers(message *socket.SendMessage) error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if m, ok := b.topicRelevance[message.Topic]; ok {
		userId := string(message.Data)
		for _, v := range m {
			if v.UserId == userId {
				v.Close()
				return nil
			}
		}
	}
	return ErrorTopicEmpty
}
