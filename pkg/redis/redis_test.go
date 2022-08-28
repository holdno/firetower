package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/go-redis/redis/v9"
	"github.com/holdno/firetower/pkg/utils"
)

func Test_RedisHSet(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	topic1 := "firetower.room.1"
	topic2 := "firetower.room.2"
	topic3 := "firetower.room.3"
	for i := 0; i < 1000; i++ {
		ip := utils.RandStringRunes(15)
		res := rdb.HMSet(context.TODO(), "ft_topics_conn_"+ip, topic1, 2, topic3, 4, topic2, 200)
		if res.Err() != nil {
			t.Fatal(res.Err())
		}

		// res := rdb.HSet(context.TODO(), "ft_cluster_clients_"+ip,)
	}
}

func Test_RedisKeyGet(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	res := rdb.Keys(context.TODO(), "ft_topics_conn_*")
	if res.Err() != nil {
		t.Fatal(res.Err())
	}

	for _, v := range res.Val() {
		fmt.Println(v)
	}
}

func Test_RedisHGet(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	res := rdb.Keys(context.TODO(), "ft_topics_conn_*")
	if res.Err() != nil {
		t.Fatal(res.Err())
	}

	topic := "firetower.room.2"
	var total int64
	for _, v := range res.Val() {
		res := rdb.HGet(context.Background(), v, topic)
		if res.Err() != nil {
			t.Fatal(res.Err())
		}

		count, err := res.Int64()
		if err != nil {
			t.Fatal(err)
		}

		total += count
	}
	t.Log(total)
}

func Test_RedisGetConnKeys(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	key := "ft_cluster_conn_*"
	res := rdb.Keys(context.Background(), key)
	if res.Err() != nil {
		t.Fatal(res.Err())
	}

	t.Log(res.Result())
}

func Test_RedisDisposeClientDown(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	for i := 0; i < 1000; i++ {
		ip := utils.RandStringRunes(15)
		value := map[string]interface{}{
			"conn":    i,
			"expired": i,
		}

		v, _ := json.Marshal(value)
		rdb.HSet(context.Background(), "ft_cluster_conn", ip, string(v))
	}

	res := rdb.HGetAll(context.Background(), "ft_cluster_conn")
	if res.Err() != nil {
		t.Fatal(res.Err())
	}

	data := res.Val()
	for ip, value := range data {
		v := make(map[string]interface{})
		json.Unmarshal([]byte(value), &v)

		if v["expired"].(float64) > 800 {
			fmt.Println("delete", ip)
			res := rdb.HDel(context.Background(), "ft_cluster_conn", ip)
			if res.Err() != nil {
				t.Fatal(res.Err())
			}
		}
	}
}

func Test_RedisIncrBy(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	// ip := utils.RandStringRunes(15)
	// key := "ft_cluster_conn_" + ip
	msgChan := make(chan struct{}, 50000)
	key := "ft_cluster_conn_NstsAslYFIUyEsg"
	res := rdb.Del(context.Background(), key)
	if res.Err() != nil {
		t.Fatal(res.Err())
	}

	w := sync.WaitGroup{}
	w.Add(1)
	go func() {
		for i := 0; i < 1000000; i++ {
			msgChan <- struct{}{}
		}

	}()

	ticker := time.NewTicker(time.Millisecond * 500)

	var currentCount int64 = 0
	var total int64 = 0
	go func() {
		for {
			select {
			case <-ticker.C:
				fmt.Println(time.Now())
				if currentCount > 0 {
					total += currentCount
					res := rdb.IncrBy(context.Background(), key, currentCount)
					if res.Err() != nil {
						panic(res.Err())
					}
					if total >= 1000000 {
						fmt.Println("done")
						w.Done()
						return
					}
				}

			case <-msgChan:
				currentCount += 1
			}
		}
	}()

	w.Wait()
	res1 := rdb.Get(context.Background(), key)
	if res1.Err() != nil {
		t.Fatal(res1.Err())
	}

	t.Log(res1.Int64())
}
