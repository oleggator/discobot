package main

import (
	"context"
	"fmt"
)

type Queue[T any] struct {
	playQueue chan T
}

func NewQueue[T any]() Queue[T] {
	return Queue[T]{
		playQueue: make(chan T, 32),
	}
}

func (pq *Queue[T]) Add(item T) error {
	select {
	case pq.playQueue <- item:
	default:
		return fmt.Errorf("queue is full")
	}
	return nil
}

func (pq *Queue[T]) Get(ctx context.Context) (T, error) {
	var item T
	select {
	case item = <-pq.playQueue:
	case <-ctx.Done():
	}
	return item, ctx.Err()
}

func (pq *Queue[T]) Clean() {

}

func (pq *Queue[T]) Len() int {
	return len(pq.playQueue)
}

func (pq *Queue[T]) String() string {
	return ""
}
