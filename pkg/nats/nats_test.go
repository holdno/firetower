package nats

import (
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

func Test_Nats(t *testing.T) {
	// TODO config
	nc, err := nats.Connect("nats://172.18.153.74:4222", nats.Name("FireTower"), nats.UserInfo("firetower", "firetower"))
	if err != nil {
		log.Fatal(err)
	}

	topic := "msg.test"

	go func() {
		_, err := nc.Subscribe(topic, func(msg *nats.Msg) {
			fmt.Println("?????")
			fmt.Println(string(msg.Data))
		})
		if err != nil {
			panic(err)
		}

	}()

	time.Sleep(time.Second)
	if err = nc.Publish(topic, []byte("hello")); err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second * 5)
}
