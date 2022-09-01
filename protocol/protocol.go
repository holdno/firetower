package protocol

import (
	"strconv"
	"sync"
	"time"

	"github.com/holdno/firetower/utils"

	json "github.com/json-iterator/go"
)

// type Message struct {
// 	Ctx   MessageContext `json:"c"`
// 	Data  []byte         `json:"d"`
// 	Topic string         `json:"t"`
// }

// type MessageContext struct {
// 	ID       string `json:"i"`
// 	MsgTime  int64  `json:"m"`
// 	Source   string `json:"s"`
// 	PushType int    `json:"p"`
// 	Type     string `json:"t"`
// }

type Coder interface {
	Decode([]byte) (*FireInfo, error)
	Encode(msg *FireInfo) []byte
}

type DefaultCoder struct{}

func (c *DefaultCoder) Decode(data []byte) (msg *FireInfo, err error) {
	err = json.Unmarshal(data, msg)
	return
}

func (c *DefaultCoder) Encode(msg *FireInfo) []byte {
	raw, _ := json.Marshal(msg)
	return raw
}

var (
	firePool sync.Pool
	// FireLogger 接管链接log t log类型 info log信息
	FireLogger func(f *FireInfo, types, info string)
)

func init() {
	firePool.New = func() interface{} {
		return &FireInfo{
			Context: new(FireLife),
			Message: new(TopicMessage),
		}
	}

	FireLogger = fireLog
}

// FireInfo 接收的消息结构体
type FireInfo struct {
	Context     *FireLife     `json:"c"`
	MessageType int           `json:"t"`
	Message     *TopicMessage `json:"m"`
}

// TopicMessage 话题信息结构体
type TopicMessage struct {
	Topic string          `json:"topic"`
	Data  json.RawMessage `json:"data"` // 可能是个json
	Type  string          `json:"type"`
}

type TowerInfo interface {
	UserID() string
	ClientID() string
}

// NewFireInfo 第二个参数的作用是继承
// 继承上一个消息体的上下文，方便日志追踪或逻辑统一
func NewFireInfo(tower TowerInfo) *FireInfo {
	fireInfo := firePool.Get().(*FireInfo)
	fireInfo.Message.Type = PublishKey
	fireInfo.Context.reset("logic", tower.ClientID(), tower.UserID())
	return fireInfo
}

// Recycling 变量回收
func (f *FireInfo) Recycling() {
	firePool.Put(f)
}

// Panic 消息的panic日志 并回收变量
func (f *FireInfo) Panic(info string) {
	FireLogger(f, "Panic", info)
	f.Recycling()
}

// Info 记录一个INFO级别的日志
func (f *FireInfo) Info(info string) {
	FireLogger(f, "INFO", info)
}

// Error 记录一个ERROR级别的日志
func (f *FireInfo) Error(info string) {
	FireLogger(f, "ERROR", info)
}

// FireLife 客户端推送消息的结构体
type FireLife struct {
	ID        string            `json:"i"`
	StartTime time.Time         `json:"t"`
	ClientID  string            `json:"c"`
	UserID    string            `json:"u"`
	Source    string            `json:"s"`
	ExtMeta   map[string]string `json:"e,omitempty"`
}

func (f *FireLife) reset(source, clientID, userID string) {
	f.StartTime = time.Now()
	f.ID = strconv.FormatInt(utils.IDWorker().GetId(), 10)
	f.ClientID = clientID
	f.UserID = userID
	f.Source = source
}
