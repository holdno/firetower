package redis

import (
	"context"
	"testing"
	"time"

	"github.com/go-redis/redis/v9"
	"github.com/holdno/firetower/utils"
	"github.com/vmihailenco/msgpack/v5"
)

func TestRedis_HSet(t *testing.T) {
	a := connNumTmp{
		N: 19999,
		T: time.Now().Unix(),
	}
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	b, err := msgpack.Marshal(&a)
	if err != nil {
		t.Fatal(err)
	}

	ip, err := utils.GetIP()
	if err != nil {
		t.Fatal(err)
	}
	res := rdb.HSet(context.Background(), ClusterConnKey, ip, string(b))
	if res.Err() != nil {
		t.Fatal(res.Err())
	}

	gRes := rdb.HGet(context.Background(), ClusterConnKey, ip)
	if gRes.Err() != nil {
		t.Fatal(gRes.Err())
	}

	t.Log(gRes.Val())

	var na connNumTmp
	if err := msgpack.Unmarshal([]byte(gRes.Val()), &na); err != nil {
		t.Fatal(err)
	}

	if na != a {
		t.Fatal(na)
	}
	t.Log("successful")
}
