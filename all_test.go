package all_test

import (
	"sync"
	"testing"

	"github.com/snowmerak/all"
)

const (
	Subscribers = 1000
	Messages    = 10000
	Buffer      = 100
)

func BenchmarkAllLinkedList(b *testing.B) {
	ll := all.New[[]byte](Buffer)

	wg := sync.WaitGroup{}
	for i := 0; i < Subscribers; i++ {
		wg.Add(1)
		ll.Subscribe(func(value []byte, closed bool) {
			if closed {
				wg.Done()
			}

			mesg := value
			_ = mesg
		})
	}

	for i := 0; i < Messages; i++ {
		ll.Push([]byte("hello"))
	}
	ll.Close()

	wg.Wait()
}

func BenchmarkChannel(b *testing.B) {
	chList := make([]chan []byte, Subscribers)

	wg := sync.WaitGroup{}
	for i := 0; i < Subscribers; i++ {
		wg.Add(1)
		chList[i] = make(chan []byte, Buffer)
		go func(ch chan []byte) {
			defer wg.Done()
			for mesg := range ch {
				_ = mesg
			}
		}(chList[i])
	}

	for i := 0; i < Messages; i++ {
		for j := 0; j < Subscribers; j++ {
			chList[j] <- []byte("hello")
		}
	}
	for i := 0; i < Subscribers; i++ {
		close(chList[i])
	}

	wg.Wait()
}
