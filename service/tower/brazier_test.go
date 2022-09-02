package tower

import (
	"fmt"
	"sync"
	"testing"
)

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
