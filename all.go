package all

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/RussellLuo/timingwheel"
)

type FakeLock struct{}

func (fl FakeLock) Lock() {}

func (fl FakeLock) Unlock() {}

type Node[T any] struct {
	Value  T
	Next   *Node[T]
	Closed bool
}

type AsyncLinkedList[T any] struct {
	head               atomic.Pointer[Node[T]]
	tail               atomic.Pointer[Node[T]]
	bufferLength       atomic.Int64
	maxBuf             int64
	lock               sync.Locker
	cond               *sync.Cond
	wheel              *timingwheel.TimingWheel
	latestNotification time.Time
}

func (all *AsyncLinkedList[T]) getNode() *Node[T] {
	return new(Node[T])
}

func (all *AsyncLinkedList[T]) putNode(_ *Node[T]) {
}

func New[T any](maxBufferSize int64) *AsyncLinkedList[T] {
	l := FakeLock{}
	tw := timingwheel.NewTimingWheel(50*time.Millisecond, 60)
	all := &AsyncLinkedList[T]{
		maxBuf: maxBufferSize,
		lock:   l,
		cond:   sync.NewCond(l),
		wheel:  tw,
	}
	tw.ScheduleFunc(all, func() {
		if all.latestNotification.Add(10 * time.Millisecond).Before(time.Now()) {
			all.latestNotification = time.Now()
			all.cond.Broadcast()
		}
	})
	tw.Start()
	return all
}

func (all *AsyncLinkedList[T]) Next(c time.Time) time.Time {
	m := c
	if all.latestNotification.After(c) {
		m = all.latestNotification
	}
	return m.Add(10 * time.Millisecond)
}

func (all *AsyncLinkedList[T]) Push(value T) {
	all.push(value, false)
}

func (all *AsyncLinkedList[T]) Close() {
	all.push(*new(T), true)
}

func (all *AsyncLinkedList[T]) push(value T, closed bool) {
	node := all.getNode()
	node.Value = value
	node.Closed = closed

outer:
	for {
		tail := all.tail.Load()
		if all.tail.CompareAndSwap(tail, node) {
			switch tail {
			case nil:
				all.head.Store(node)
			default:
				tail.Next = node
			}
			all.latestNotification = time.Now()
			all.cond.Broadcast()
			if all.bufferLength.Add(1) > all.maxBuf {
				all.bufferLength.Add(-1)
			inner:
				for {
					head := all.head.Load()
					if all.head.CompareAndSwap(head, head.Next) {
						all.putNode(head)
						break inner
					}
					fmt.Println("failed to swap head")
				}
			}
			break outer
		}
	}
}

func (all *AsyncLinkedList[T]) Subscribe(callback func(value T, closed bool)) <-chan struct{} {
	cursor := all.head.Load()
	last := (*Node[T])(nil)
	closed := make(chan struct{}, 1)

	go func() {
		for {
			all.lock.Lock()
			all.cond.Wait()
			all.lock.Unlock()

			if cursor == nil {
				switch last {
				case nil:
					cursor = all.head.Load()
				default:
					cursor = last.Next
				}
			}

			for cursor != nil {
				last = cursor
				callback(cursor.Value, cursor.Closed)
				switch cursor.Closed {
				case true:
					close(closed)
					return
				}
				cursor = cursor.Next
			}
		}
	}()

	return closed
}
