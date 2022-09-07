package redis

import (
	"context"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/go-redis/redis/v9"
	"github.com/holdno/firetower/utils"
)

func TestRedis_HSet(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	ip, err := utils.GetIP()
	if err != nil {
		t.Fatal(err)
	}
	res := rdb.HSet(context.Background(), ClusterConnKey, ip, fmt.Sprintf("%d|%d", 2315, time.Now().Unix()))
	if res.Err() != nil {
		t.Fatal(res.Err())
	}

	gRes := rdb.HGet(context.Background(), ClusterConnKey, ip)
	if gRes.Err() != nil {
		t.Fatal(gRes.Err())
	}

	t.Log(gRes.Val())

	if gRes.Val() != fmt.Sprintf("%d|%d", 2315, time.Now().Unix()) {
		t.Fatal(gRes.Val())
	}
	t.Log("successful")
}

func TestLua(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	res1 := rdb.Del(context.TODO(), ClusterConnKey)
	if res1.Err() != nil {
		t.Fatal(res1.Err())
	}

	b := make([]byte, 16)
	binary.LittleEndian.PutUint64(b, 2315)
	binary.LittleEndian.PutUint64(b[8:], uint64(time.Now().Unix()-10))
	res := rdb.HSet(context.TODO(), ClusterConnKey, "localhost", string(b))
	if res.Err() != nil {
		t.Fatal(res.Err())
	}

	result := rdb.Eval(context.TODO(), clusterShutdownCheckerScript, []string{ClusterConnKey, fmt.Sprintf("%d", time.Now().Unix())}, 2)
	if result.Err() != nil {
		t.Fatal(result.Err())
	}
	t.Log(result.Val())
}

func TestPack(t *testing.T) {
	var number uint64 = 3512
	r := packClientConnNumberNow(number)
	n, err := unpackClientConnNumberNow(r + "1")
	if err != nil {
		t.Fatal(err)
	}

	if n != number {
		t.Fatal(n, number)
	}
	t.Log("successful")
}

func TestLock(t *testing.T) {
	lockKey := "ft_cluster_master"
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	locker := func(clusterID string) {
		for {
			res := rdb.SetNX(context.Background(), lockKey, clusterID, time.Second*3)
			if res.Val() {
				for {
					ticker := time.NewTicker(time.Second * 1)
					select {
					case <-ticker.C:
						res := rdb.Eval(context.Background(), keepMasterScript, []string{lockKey, clusterID}, 2)
						if res.Val() != "success" || res.Err() != nil {
							ticker.Stop()
							break
						}
						fmt.Println(clusterID, "expire")
					}
				}
			}
			time.Sleep(time.Second)
		}
	}

	go locker("1")
	go locker("2")
	go locker("3")
	go locker("4")

	time.Sleep(time.Second * 20)
}
