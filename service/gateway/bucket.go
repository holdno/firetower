package gateway

import (
	"errors"
	"github.com/holdno/firetower/socket"
	"sync"
	"sync/atomic"
)

var TM *TowerManager

var (
	ErrorTopicEmpty = errors.New("topic is empty")
)

type TowerManager struct {
	bucket      []*Bucket
	centralChan chan *socket.SendMessage // 中心处理队列
}

type Bucket struct {
	mu             sync.RWMutex // 读写锁，可并发读不可并发读写
	id             int64
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
			}
		}
	}()
}

func newBucket() *Bucket {
	b := &Bucket{
		id:             getNewBucketId(),
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

func (t *TowerManager) GetBucket(bt *FireTower) (bucket *Bucket) {
	bucket = t.bucket[bt.connId%uint64(len(t.bucket))]
	return
}

func (b *Bucket) consumer() {
	for {
		select {
		case message := <-b.BuffChan:
			b.Push(message)
		}
	}
}

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

// 桶内进行遍历push
func (b *Bucket) Push(message *socket.SendMessage) error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if m, ok := b.topicRelevance[message.Topic]; ok {
		for _, v := range m {
			v.Send(message)
		}
		return nil
	} else {
		return ErrorTopicEmpty
	}
}
