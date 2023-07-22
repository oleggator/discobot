package main

import (
	"context"
	"errors"
)

type PlayStatus int

const (
	IdlePlayStatus PlayStatus = iota
	PlayingPlayStatus
	PausedPlayStatus
)

type Playback struct {
	playStatus    PlayStatus
	startPlayback chan struct{}
	skipCurrent   chan struct{}
}

func NewPlayback() Playback {
	return Playback{
		playStatus:    IdlePlayStatus,
		startPlayback: make(chan struct{}),
		skipCurrent:   make(chan struct{}, 1),
	}
}

func (pb *Playback) Resume() {
	if pb.playStatus != PausedPlayStatus {
		return
	}

	pb.playStatus = PlayingPlayStatus
	select {
	case pb.startPlayback <- struct{}{}:
	default:
		// Skip if nobody is waiting
	}
}

func (pb *Playback) StartCurrentTrack() {
	pb.playStatus = PlayingPlayStatus
}

func (pb *Playback) FinishCurrentTrack() {
	pb.playStatus = IdlePlayStatus
}

func (pb *Playback) Pause() {
	if pb.playStatus != PlayingPlayStatus {
		return
	}

	pb.playStatus = PausedPlayStatus
}

func (pb *Playback) Skip() {
	if pb.playStatus == IdlePlayStatus {
		return
	}

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

	if pb.playStatus != PlayingPlayStatus {
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
