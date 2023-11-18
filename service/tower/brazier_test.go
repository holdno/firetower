package tower

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"testing"
)

func TestConnID(t *testing.T) {
	connId = math.MaxUint64

	atomic.AddUint64(&connId, 1)
	if connId != 0 {
		t.Fatal("wrong connid")
	}
	atomic.AddUint64(&connId, 1)
	if connId != 1 {
		t.Fatal("wrong connid")
	}
}

func TestSyncPool(t *testing.T) {
	var pool = sync.Pool{
		New: func() interface{} {
			fmt.Println("new")
			return &struct{}{}
		},
	}
	n := &struct{ Name string }{
		Name: "hhhh",
	}

	pool.Put(n)
	pool.Put(n)
	pool.Put(n)
	pool.Put(n)

	a := pool.Get()
	b := pool.Get()

	a.(*struct{ Name string }).Name = "123"
	fmt.Println(b.(*struct{ Name string }).Name)

	pool.Get()
	pool.Get()
	pool.Get()
}
