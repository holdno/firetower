package protocol

import (
	"strconv"
	"time"

	json "github.com/json-iterator/go"

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

type Coder[T any] interface {
	Decode([]byte, *FireInfo[T]) error
	Encode(msg *FireInfo[T]) []byte
}

type WebSocketMessage struct {
	MessageType int
	Data        []byte
}

// FireInfo 接收的消息结构体
type FireInfo[T any] struct {
	Context     FireLife        `json:"c"`
	MessageType int             `json:"t"`
	Message     TopicMessage[T] `json:"m"`
}

func (f *FireInfo[T]) Copy() FireInfo[T] {
	return *f
}

func (f *FireInfo[T]) GetContext() FireLife {
	return f.Context
}

func (f *FireInfo[T]) GetMessage() TopicMessage[T] {
	return f.Message
}

type ReadOnlyFire[T any] interface {
	GetContext() FireLife
	GetMessage() TopicMessage[T]
	Copy() FireInfo[T]
}

// TopicMessage 话题信息结构体
type TopicMessage[T any] struct {
	Topic string        `json:"topic"`
	Data  T             `json:"data"` // 可能是个json
	Type  FireOperation `json:"type"`
}

func (s *TopicMessage[T]) Json() []byte {
	raw, _ := json.Marshal(s)
	return raw
}

type TowerInfo interface {
	UserID() string
	ClientID() string
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
