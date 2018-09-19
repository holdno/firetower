package main

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/holdno/firetower/service/gateway"
	"github.com/holdno/snowFlakeByGo"
	"net/http"
	_ "net/http/pprof"
	"strconv"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type messageInfo struct {
	From string `json:"from"`
	Data string `json:"data"`
	Type string `json:"type"`
}

var GlobalIdWorker *snowFlakeByGo.Worker

func main() {
	GlobalIdWorker, _ = snowFlakeByGo.NewWorker(1)
	// 如果是集群环境  一定一定要给每个服务设置唯一的id
	// 取值范围 1-1024
	gateway.ClusterId = 1
	http.HandleFunc("/ws", Websocket)
	fmt.Println("websocket service start: 0.0.0.0:9999")
	http.ListenAndServe("0.0.0.0:9999", nil)
}

func Websocket(w http.ResponseWriter, r *http.Request) {
	// 做用户身份验证

	// 验证成功才升级连接
	ws, _ := upgrader.Upgrade(w, r, nil)

	id := GlobalIdWorker.GetId()
	tower := gateway.BuildTower(ws, strconv.FormatInt(id, 10))

	tower.SetReadHandler(func(fire *gateway.FireInfo) bool {
		// 做发送验证
		// 判断发送方是否有权限向到达方发送内容
		tower.Publish(fire)
		return true
	})

	tower.SetReadTimeoutHandler(func(fire *gateway.FireInfo) {
		messageInfo := new(messageInfo)
		err := json.Unmarshal(fire.Message.Data, messageInfo)
		if err != nil {
			return
		}
		messageInfo.Type = "timeout"
		b, _ := json.Marshal(messageInfo)
		err = tower.ToSelf(b)
		if err != gateway.ErrorClose {
			fmt.Println("err:", err)
		}
	})

	tower.SetBeforeSubscribeHandler(func(topic []string) bool {
		// 这里用来判断当前用户是否允许订阅该topic
		return true
	})

	tower.SetSubscribeHandler(func(topic []string) bool {
		for _, v := range topic {
			num := tower.GetConnectNum(v)

			var pushmsg = gateway.GetFireInfo(tower)
			pushmsg.Message.Topic = v
			pushmsg.Message.Data = []byte(fmt.Sprintf("{\"type\":\"onSubscribe\",\"data\":%d}", num))
			tower.Publish(pushmsg)
		}
		return true
	})

	tower.SetUnSubscribeHandler(func(topic []string) bool {
		for _, v := range topic {
			num := tower.GetConnectNum(v)
			var pushmsg = gateway.GetFireInfo(tower)
			pushmsg.Message.Topic = v
			pushmsg.Message.Data = []byte(fmt.Sprintf("{\"type\":\"onUnsubscribe\",\"data\":%d}", num))
			tower.Publish(pushmsg)
		}
		return true
	})

	tower.Run()
}
