package gateway

import (
	"fmt"
	"strconv"
	"testing"
)

func TestBuildTower(t *testing.T) {
	tower := BuildTower(ws, strconv.FormatInt(1231231231, 10))

	tower.SetReadHandler(func(message *TopicMessage) bool {
		fmt.Println(message.Data)
		// 做发送验证
		// 判断发送方是否有权限向到达方发送内容
		tower.Publish(message)
		return true
	})

	tower.SetBeforeSubscribeHandler(func(topic []string) bool {
		// 这里用来判断当前用户是否允许订阅该topic
		return true
	})

	tower.SetSubscribeHandler(func(topic []string) bool {
		for _, v := range topic {
			num := tower.GetConnectNum(v)

			var pushmsg = new(TopicMessage)
			pushmsg.Topic = v
			pushmsg.Data = []byte(fmt.Sprintf("{\"type\":\"onSubscribe\",\"data\":%d}", num))
			tower.Publish(pushmsg)
		}

		return true
	})
	tower.SetUnSubscribeHandler(func(topic []string) bool {
		for _, v := range topic {
			num := tower.GetConnectNum(v)
			var pushmsg = new(TopicMessage)
			pushmsg.Topic = v
			pushmsg.Data = []byte(fmt.Sprintf("{\"type\":\"onUnsubscribe\",\"data\":%d}", num))
			tower.Publish(pushmsg)
		}

		return true
	})
	fmt.Println("new websocket running")
	tower.Run()
}
