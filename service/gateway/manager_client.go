package gateway

import (
	"fmt"
	pb "github.com/holdno/firetower/grpc/manager"
	"github.com/holdno/firetower/socket"
	"google.golang.org/grpc"
	"time"
)

func BuildManagerClient(configPath string) {
	go func() {

	Retry:
		var err error
		conn, err := grpc.Dial(ConfigTree.Get("grpc.address").(string), grpc.WithInsecure())
		if err != nil {
			fmt.Println("[manager client] grpc connect error:", ConfigTree.Get("topicServiceAddr").(string), err)
			time.Sleep(time.Duration(1) * time.Second)
			goto Retry
		}
		TopicManageGrpc = pb.NewTopicServiceClient(conn)
		topicManage = socket.NewClient(ConfigTree.Get("topicServiceAddr").(string))

		if err != nil {
			panic(fmt.Sprintf("[manager client] can not get local IP, error:%v", err))
		}
		topicManage.OnPush(func(topic string, message []byte) {
			TM.centralChan <- &SendMessage{
				MessageType: 1,
				Data:        message,
				Topic:       topic,
			}
		})
		err = topicManage.Connect()
		if err != nil {
			fmt.Println("[manager client] wait topic manager online", ConfigTree.Get("topicServiceAddr").(string))
			time.Sleep(time.Duration(1) * time.Second)
			goto Retry
		} else {
			fmt.Println("[manager client] connected:", ConfigTree.Get("topicServiceAddr").(string))
		}
	}()
}
