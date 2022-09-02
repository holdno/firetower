package nats

import (
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

func TestNats(t *testing.T) {
	nc, err := nats.Connect("nats://localhost:4222", nats.Name("FireTower"), nats.UserInfo("firetower", "firetower"))
	if err != nil {
		log.Fatal(err)
	}

	topic := "chat.world."

	go func() {
		nc.Subscribe(topic+">", func(msg *nats.Msg) {
			fmt.Println("received", string(msg.Data))
		})
	}()

	time.Sleep(time.Second)
	if err := nc.Publish(topic+"1", []byte(`{"message":"hello"}`)); err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second * 10)
}
