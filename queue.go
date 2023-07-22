package discobot

import (
	"context"
	"fmt"
)

type Queue[T any] struct {
	playQueue chan T
	capacity  int
}

func NewQueue[T any](capacity int) Queue[T] {
	queue := Queue[T]{capacity: capacity}
	queue.Clean()
	return queue
}

func (pq *Queue[T]) Push(item T) error {
	select {
	case pq.playQueue <- item:
	default:
		return fmt.Errorf("queue is full")
	}
	return nil
}

func (pq *Queue[T]) Pop(ctx context.Context) (T, error) {
	for {
		select {
		case <-ctx.Done():
			var empty T
			return empty, ctx.Err()

		case item, ok := <-pq.playQueue:
			if !ok {
				continue
			}

			return item, nil
		}
	}
}

func (pq *Queue[T]) Clean() {
	oldChan := pq.playQueue
	pq.playQueue = make(chan T, pq.capacity)

	if oldChan != nil {
		close(oldChan)
	}
}

func (pq *Queue[T]) Len() int {
	return len(pq.playQueue)
}
