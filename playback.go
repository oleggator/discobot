package main

import (
	"context"
	"errors"
)

type Playback struct {
	playStatus    bool
	startPlayback chan struct{}
	skipCurrent   chan struct{}
}

func NewPlayback() Playback {
	return Playback{
		playStatus:    true,
		startPlayback: make(chan struct{}),
		skipCurrent:   make(chan struct{}, 1),
	}
}

func (pb *Playback) Play() {
	pb.playStatus = true
	select {
	case pb.startPlayback <- struct{}{}:
	default:
		// Skip if nobody is waiting
	}
}

func (pb *Playback) Pause() {
	pb.playStatus = false
}

func (pb *Playback) Skip() {
	select {
	case pb.skipCurrent <- struct{}{}:
	default:
		// Skip if nobody is waiting
	}
}

func (pb *Playback) Check(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if !pb.playStatus {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-pb.startPlayback:
		case <-pb.skipCurrent:
			return errors.New("track is skipped")
		}
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-pb.skipCurrent:
		return errors.New("track is skipped")
	default:
	}

	return nil
}
