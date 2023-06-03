package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/holdno/firetower/config"
	"github.com/holdno/firetower/protocol"
	"github.com/holdno/firetower/service/tower"
	towersvc "github.com/holdno/firetower/service/tower"
	"github.com/holdno/firetower/utils"
	"github.com/holdno/snowFlakeByGo"
	json "github.com/json-iterator/go"
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

var _ tower.PusherInfo = (*SystemPusher)(nil)

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

func main() {
	// 全局唯一id生成器
	tm, err := towersvc.Setup(config.FireTowerConfig{
		WriteChanLens: 1000,
		Heartbeat:     30,
		ServiceMode:   config.SingleMode,
		Bucket: config.BucketConfig{
			Num:              4,
			CentralChanCount: 100000,
			BuffChanCount:    1000,
			ConsumerNum:      1,
		},
		// Cluster: config.Cluster{
		// 	RedisOption: config.Redis{
		// 		Addr: "localhost:6379",
		// 	},
		// 	NatsOption: config.Nats{
		// 		Addr:       "nats://localhost:4222",
		// 		UserName:   "firetower",
		// 		Password:   "firetower",
		// 		ServerName: "firetower",
		// 	},
		// },
	})

	if err != nil {
		panic(err)
	}

	systemer = &SystemPusher{
		clientID: "1",
	}

	go func() {
		for {
			time.Sleep(time.Second * 60)
			f := tower.NewFire(protocol.SourceSystem, systemer)
			f.Message.Topic = "/chat/world"
			f.Message.Data = []byte(fmt.Sprintf("{\"type\":\"publish\",\"data\":\"请通过 room 命令切换聊天室\",\"from\":\"system\"}"))
			tm.Publish(f)
		}
	}()

	http.HandleFunc("/ws", Websocket)
	fmt.Println("websocket service start: 0.0.0.0:9999")
	http.ListenAndServe("0.0.0.0:9999", nil)
}

// Websocket http转websocket连接 并实例化firetower
func Websocket(w http.ResponseWriter, r *http.Request) {
	// 做用户身份验证

	// 验证成功才升级连接
	ws, _ := upgrader.Upgrade(w, r, nil)

	id := utils.IDWorker().GetId()
	tower, err := towersvc.BuildTower(ws, strconv.FormatInt(id, 10))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	tower.SetReadHandler(func(fire *protocol.FireInfo) bool {
		// fire将会在handler执行结束后被回收
		messageInfo := new(messageInfo)
		err := json.Unmarshal(fire.Message.Data, messageInfo)
		if err != nil {
			return false
		}
		msg := strings.Trim(string(messageInfo.Data), "\"")
		switch true {
		case strings.HasPrefix(msg, "/name "):
			tower.SetUserID(strings.TrimLeft(msg, "/name "))
			messageInfo.From = "system"
			messageInfo.Data = []byte(fmt.Sprintf(`{"type": "change_name", "name": "%s"}`, tower.UserID()))
			messageInfo.Type = "event"
			fire.Message.Data, _ = json.Marshal(messageInfo)
			tower.ToSelf(fire.Message.Data)
			return false
		case strings.HasPrefix(msg, "/room "):
			if err = tower.UnSubscribe(fire.Context, tower.TopicList()); err != nil {
				messageInfo.From = "system"
				messageInfo.Type = "event"
				messageInfo.Data = []byte(fmt.Sprintf(`{"type": "error", "msg": "切换房间失败, %s"}`, err.Error()))
				fire.Message.Data, _ = json.Marshal(messageInfo)
				tower.ToSelf(fire.Message.Data)
				return false
			}
			roomCode := strings.TrimLeft(msg, "/room ")
			if err = tower.Subscribe(fire.Context, []string{"/chat/" + roomCode}); err != nil {
				messageInfo.From = "system"
				messageInfo.Type = "event"
				messageInfo.Data = []byte(fmt.Sprintf(`{"type": "error", "msg": "切换房间失败, %s, 请重新尝试"}`, err.Error()))
				fire.Message.Data, _ = json.Marshal(messageInfo)
				tower.ToSelf(fire.Message.Data)
				return false
			}

			return false
		}

		if tower.UserID() == "" {
			tower.SetUserID(messageInfo.From)
		}
		messageInfo.From = tower.UserID()
		fire.Message.Data, _ = json.Marshal(messageInfo)
		// 做发送验证
		// 判断发送方是否有权限向到达方发送内容
		return true
	})

	tower.SetReceivedHandler(func(fi *protocol.FireInfo) bool {
		return true
	})

	tower.SetReadTimeoutHandler(func(fire *protocol.FireInfo) {
		messageInfo := new(messageInfo)
		err := json.Unmarshal(fire.Message.Data, messageInfo)
		if err != nil {
			return
		}
		messageInfo.Type = "timeout"
		b, _ := json.Marshal(messageInfo)
		err = tower.ToSelf(b)
		if err != towersvc.ErrorClose {
			fmt.Println("err:", err)
		}
	})

	tower.SetBeforeSubscribeHandler(func(context protocol.FireLife, topic []string) bool {
		// 这里用来判断当前用户是否允许订阅该topic
		return true
	})

	tower.SetSubscribeHandler(func(context protocol.FireLife, topic []string) bool {
		for _, v := range topic {
			if strings.HasPrefix(v, "/chat/") {
				roomCode := strings.TrimPrefix(v, "/chat/")
				messageInfo := new(messageInfo)
				messageInfo.From = "system"
				messageInfo.Type = "event"
				messageInfo.Data = []byte(fmt.Sprintf(`{"type": "change_room", "room": "%s"}`, roomCode))
				msg, _ := json.Marshal(messageInfo)
				tower.ToSelf(msg)
				return true
			}
		}
		return true
	})

	tower.SetUnSubscribeHandler(func(context protocol.FireLife, topic []string) bool {
		// for _, v := range topic {
		// 	num := tower.GetConnectNum(v)
		// 	var pushmsg = protocol.NewFireInfo(tower)
		// 	pushmsg.Message.Topic = v
		// 	pushmsg.Message.Data = []byte(fmt.Sprintf("{\"type\":\"onUnsubscribe\",\"data\":%d}", num))
		// 	tower.Publish(pushmsg)
		// }
		return true
	})

	ticker := time.NewTicker(time.Millisecond * 500)
	go func() {
		topicConnCache := make(map[string]uint64)
		for {
			select {
			case <-tower.OnClose():
				return
			case <-ticker.C:
				for _, v := range tower.TopicList() {
					num, err := tower.GetConnectNum(v)
					if err != nil {
						tower.Logger().Error("failed to get connect number", zap.Error(err))
						continue
					}
					if topicConnCache[v] == num {
						continue
					}

					tower.ToSelf([]byte(fmt.Sprintf("{\"type\":\"onSubscribe\",\"data\":%d}", num)))
					topicConnCache[v] = num
				}
			}
		}
	}()

	tower.Run()
}
