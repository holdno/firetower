package main

import (
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/holdno/beacon/gateway"
	"net/http"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func main() {
	http.HandleFunc("/ws", Websocket)
	fmt.Println("websocket service start: 0.0.0.0:9999")
	http.ListenAndServe("0.0.0.0:9999", nil)
}

func Websocket(w http.ResponseWriter, r *http.Request) {
	// 做用户身份验证

	// 验证成功才升级连接
	ws, _ := upgrader.Upgrade(w, r, nil)

	tower := gateway.BuildTower(ws)

	tower.SetReadHandler(func(message *gateway.TopicMessage) bool {
		fmt.Println(message.Data)
		// 做发送验证
		// 判断发送方是否有权限向到达方发送内容
		tower.Publish(message)
		return true
	})
	fmt.Println("new websocket running")
	tower.Run()
}
