package protocol

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/holdno/firetower/utils"
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
	Decode([]byte, *FireInfo) error
	Encode(msg *FireInfo) []byte
}

var (
	// FireLogger 接管链接log t log类型 info log信息
	FireLogger func(f *FireInfo, types, info string)
)

func init() {
	FireLogger = fireLog
}

type WebSocketMessage struct {
	MessageType int
	Data        []byte
}

// FireInfo 接收的消息结构体
type FireInfo struct {
	Context     FireLife     `json:"c"`
	MessageType int          `json:"t"`
	Message     TopicMessage `json:"m"`
}

func (f *FireInfo) Copy() FireInfo {
	return *f
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

// Panic 消息的panic日志 并回收变量
func (f *FireInfo) Panic(info string) {
	FireLogger(f, "Panic", info)
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
	Source    FireSource        `json:"s"`
	ExtMeta   map[string]string `json:"e,omitempty"`
}

func (f *FireLife) Reset(source FireSource, clientID, userID string) {
	f.StartTime = time.Now()
	f.ID = strconv.FormatInt(utils.IDWorker().GetId(), 10)
	f.ClientID = clientID
	f.UserID = userID
	f.Source = source
}
