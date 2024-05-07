package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/holdno/firetower/config"
	"github.com/holdno/firetower/protocol"
	towersvc "github.com/holdno/firetower/service/tower"
	"github.com/holdno/firetower/utils"
	"github.com/holdno/snowFlakeByGo"
	jsoniter "github.com/json-iterator/go"
	"go.uber.org/zap"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type messageInfo struct {
	From string          `json:"from"`
	Data json.RawMessage `json:"data"`
	Type string          `json:"type"`
}

// GlobalIdWorker 全局唯一id生成器
var GlobalIdWorker *snowFlakeByGo.Worker

var _ towersvc.PusherInfo = (*SystemPusher)(nil)

type SystemPusher struct {
	clientID string
}

func (s *SystemPusher) UserID() string {
	return "system"
}
func (s *SystemPusher) ClientID() string {
	return s.clientID
}

var systemer *SystemPusher

const (
	listenAddress = "127.0.0.1:9999"
	websocketPath = "/ws"
)

func startTower() {
	// 全局唯一id生成器
	tm, err := towersvc.Setup[jsoniter.RawMessage](config.FireTowerConfig{
		WriteChanLens: 1000,
		ReadChanLens:  1000,
		Heartbeat:     30,
		ServiceMode:   config.SingleMode,
		Bucket: config.BucketConfig{
			Num:              4,
			CentralChanCount: 100000,
			BuffChanCount:    1000,
			ConsumerNum:      2,
		},
	})

	if err != nil {
		panic(err)
	}

	systemer = &SystemPusher{
		clientID: "1",
	}

	tower := &Tower{
		tm: tm,
	}
	http.HandleFunc(websocketPath, tower.Websocket)
	tm.Logger().Info("http server start", zap.String("address", listenAddress))
	if err := http.ListenAndServe(listenAddress, nil); err != nil {
		panic(err)
	}
}

type Tower struct {
	tm towersvc.Manager[jsoniter.RawMessage]
}

const (
	bindTopic = "bindtopic"
)

// Websocket http转websocket连接 并实例化firetower
func (t *Tower) Websocket(w http.ResponseWriter, r *http.Request) {
	// 做用户身份验证

	// 验证成功才升级连接
	ws, _ := upgrader.Upgrade(w, r, nil)

	id := utils.IDWorker().GetId()
	tower, err := t.tm.BuildTower(ws, strconv.FormatInt(id, 10))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	tower.SetReadHandler(func(fire protocol.ReadOnlyFire[jsoniter.RawMessage]) bool {
		return true
	})

	tower.SetReceivedHandler(func(fi protocol.ReadOnlyFire[jsoniter.RawMessage]) bool {
		return true
	})

	tower.SetReadTimeoutHandler(func(fire protocol.ReadOnlyFire[jsoniter.RawMessage]) {
		messageInfo := new(messageInfo)
		err := json.Unmarshal(fire.GetMessage().Data, messageInfo)
		if err != nil {
			return
		}
		messageInfo.Type = "timeout"
		b, _ := json.Marshal(messageInfo)
		tower.SendToClient(b)
	})

	tower.SetBeforeSubscribeHandler(func(context protocol.FireLife, topic []string) bool {
		// 这里用来判断当前用户是否允许订阅该topic
		for _, v := range topic {
			if v == bindTopic {
				messageInfo := new(messageInfo)
				messageInfo.From = "system"
				messageInfo.Type = "event"
				messageInfo.Data = []byte(fmt.Sprintf(`{"type": "bind", "topic": "%s"}`, v))
				msg, _ := json.Marshal(messageInfo)
				tower.SendToClient(msg)
			}
		}
		return true
	})

	tower.SetSubscribeHandler(func(context protocol.FireLife, topic []string) {
		for _, v := range topic {
			messageInfo := new(messageInfo)
			messageInfo.From = "system"
			messageInfo.Type = "event"
			messageInfo.Data = []byte(fmt.Sprintf(`{"type": "subscribe", "topic": "%s"}`, v))
			msg, _ := json.Marshal(messageInfo)
			tower.SendToClient(msg)
		}
	})

	go tower.Run()

	ws.Close()

	if err := tower.SendToClientBlock([]byte("test")); err != nil {
		fmt.Println("send error", err)
	}
}

func buildClient(t *testing.T) *websocket.Conn {
	url := fmt.Sprintf("ws://%s%s", listenAddress, websocketPath)
	client, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		for {
			_, data, err := client.ReadMessage()
			if err != nil {
				return
			}

			fmt.Println("--- client receive message ---")
			fmt.Println(string(data))
		}
	}()
	return client
}

func TestBaseTower(t *testing.T) {
	go startTower()
	time.Sleep(time.Second)

	client1 := buildClient(t)
	subMsg := protocol.TopicMessage[jsoniter.RawMessage]{
		Topic: bindTopic,
		Type:  protocol.SubscribeOperation,
	}
	if err := client1.WriteMessage(websocket.TextMessage, subMsg.Json()); err != nil {
		t.Fatal(err)
	}

	subMsg.Topic = "testtopic"
	if err := client1.WriteMessage(websocket.TextMessage, subMsg.Json()); err != nil {
		t.Fatal(err)
	}

	client2 := buildClient(t)
	if err := client2.WriteMessage(websocket.BinaryMessage, subMsg.Json()); err != nil {
		t.Fatal(err)
	}

	testMessage := protocol.TopicMessage[jsoniter.RawMessage]{
		Topic: subMsg.Topic,
		Type:  protocol.PublishOperation,
		Data:  jsoniter.RawMessage([]byte("\"hi\"")),
	}

	fire := new(protocol.FireInfo[jsoniter.RawMessage]) // 从对象池中获取消息对象 降低GC压力
	fire.MessageType = 1
	if err := jsoniter.Unmarshal(testMessage.Json(), &fire.Message); err != nil {
		t.Fatal(err)
	}
	client1.WriteMessage(websocket.BinaryMessage, testMessage.Json())

	time.Sleep(time.Minute)
}
