package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"strconv"

	"github.com/gorilla/websocket"
	"github.com/OSMeteor/firetower/service/gateway"
	"github.com/holdno/snowFlakeByGo"
	json "github.com/json-iterator/go"
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

// GlobalIdWorker 全局唯一id生成器
var GlobalIdWorker *snowFlakeByGo.Worker

func main() {
	// 全局唯一id生成器
	GlobalIdWorker, _ = snowFlakeByGo.NewWorker(1)
	// 如果是集群环境  一定一定要给每个服务设置唯一的id
	// 取值范围 1-1024
	gateway.ClusterId = 1
	gateway.Init()
	http.HandleFunc("/ws", Websocket)
	fmt.Println("websocket service start: 0.0.0.0:9999")
	http.ListenAndServe("0.0.0.0:9999", nil)
}

// Websocket http转websocket连接 并实例化firetower
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

	tower.SetBeforeSubscribeHandler(func(context *gateway.FireLife, topic []string) bool {
		// 这里用来判断当前用户是否允许订阅该topic
		return true
	})

	tower.SetSubscribeHandler(func(context *gateway.FireLife, topic []string) bool {
		for _, v := range topic {
			num := tower.GetConnectNum(v)
			// 继承订阅消息的context
			var pushmsg = gateway.NewFireInfo(tower, context)
			pushmsg.Message.Topic = v
			pushmsg.Message.Data = []byte(fmt.Sprintf("{\"type\":\"onSubscribe\",\"data\":%d}", num))
			tower.Publish(pushmsg)
		}
		return true
	})

	tower.SetUnSubscribeHandler(func(context *gateway.FireLife, topic []string) bool {
		for _, v := range topic {
			num := tower.GetConnectNum(v)
			var pushmsg = gateway.NewFireInfo(tower, context)
			pushmsg.Message.Topic = v
			pushmsg.Message.Data = []byte(fmt.Sprintf("{\"type\":\"onUnsubscribe\",\"data\":%d}", num))
			fmt.Println(pushmsg)
			tower.Publish(pushmsg)
		}
		return true
	})

	tower.Run()
}
