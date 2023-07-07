package main

import (
	"context"
	"discobot/ytdlp"
	"fmt"
	"io"
	"log"

	dg "github.com/andersfylling/disgord"
	"github.com/at-wat/ebml-go"
	"github.com/at-wat/ebml-go/webm"
	"golang.org/x/sync/errgroup"
)

type DiscoBot struct {
	client            *dg.Client
	playback          Playback
	playQueue         Queue[*Task]
	channelIDByUserID map[dg.Snowflake]dg.Snowflake
}

type Task struct {
	video              *ytdlp.FetchResult
	guildID, channelID dg.Snowflake
}

func NewDiscoBot(token string) *DiscoBot {
	client := dg.New(dg.Config{
		BotToken: token,
		Intents:  dg.IntentGuilds | dg.IntentGuildMessages | dg.IntentGuildVoiceStates,
	})

	bot := &DiscoBot{
		client:            client,
		playback:          NewPlayback(),
		playQueue:         NewQueue[*Task](32),
		channelIDByUserID: make(map[dg.Snowflake]dg.Snowflake),
	}

	gateway := client.Gateway()
	gateway.GuildCreate(bot.guildCreate)
	gateway.InteractionCreate(bot.handleInteractionCreate)
	gateway.BotReady(func() {
		log.Println("bot is ready")
	})

	gateway.VoiceStateUpdate(func(s dg.Session, h *dg.VoiceStateUpdate) {
		if channelID := h.VoiceState.ChannelID; !channelID.IsZero() {
			bot.channelIDByUserID[h.VoiceState.UserID] = h.VoiceState.ChannelID
		} else {
			delete(bot.channelIDByUserID, h.VoiceState.UserID)
		}
	})

	return bot
}

func (bot *DiscoBot) Open(ctx context.Context) error {
	return bot.client.Gateway().WithContext(ctx).Connect()
}

func (bot *DiscoBot) Close() error {
	return bot.client.Gateway().Disconnect()
}

func (bot *DiscoBot) queueTrack(ctx context.Context, guildID, channelID dg.Snowflake, url string) error {
	video, err := ytdlp.Fetch(ctx, url)
	if err != nil {
		return err
	}

	if err := bot.playQueue.Push(&Task{
		video:     video,
		guildID:   guildID,
		channelID: channelID,
	}); err != nil {
		return err
	}

	return nil
}

type Segment struct {
	SeekHead *webm.SeekHead `ebml:"SeekHead"`
	Info     webm.Info      `ebml:"Info"`
	Tracks   webm.Tracks    `ebml:"Tracks"`
	Cues     *webm.Cues     `ebml:"Cues"`

	ClustersChan chan *webm.Cluster `ebml:"Cluster"`
}

type Container struct {
	Header  webm.EBMLHeader `ebml:"EBML"`
	Segment Segment         `ebml:"Segment"`
}

func (bot *DiscoBot) RunPlayer(ctx context.Context) error {
	var voice dg.VoiceConnection
	for {
		task, err := bot.playQueue.Pop(ctx)
		if err != nil {
			return err
		}

		if voice == nil {
			// Join the provided voice channel.
			voice, err = bot.client.Guild(task.guildID).VoiceChannel(task.channelID).Connect(false, true)
			if err != nil {
				return err
			}
		}

		if err := bot.play(ctx, voice, task); err != nil {
			log.Println(err)
		}

		if bot.playQueue.Len() == 0 {
			voice.Close()
			voice = nil
		}
	}
}

func (bot *DiscoBot) play(ctx context.Context, voice dg.VoiceConnection, task *Task) error {
	// cluster's size is usually below about 175 000 bytes
	clusterChan := make(chan *webm.Cluster, 32)
	r, w := io.Pipe()

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		// Start speaking.
		voice.StartSpeaking()
		defer voice.StopSpeaking()

		for cluster := range clusterChan {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			for _, block := range cluster.SimpleBlock {
				if ctx.Err() != nil {
					return ctx.Err()
				}

				for _, data := range block.Data {
					if err := bot.playback.Check(ctx); err != nil {
						return err
					}
					if err := voice.SendOpusFrame(data); err != nil {
						return err
					}
				}
			}
		}

		return nil
	})
	eg.Go(func() error {
		defer w.Close()
		if err := task.video.Download(ctx, w); err != nil {
			return err
		}

		return nil
	})
	eg.Go(func() error {
		defer close(clusterChan)
		defer r.Close()

		var container Container
		container.Segment.ClustersChan = clusterChan
		if err := ebml.Unmarshal(r, &container); err != nil {
			return fmt.Errorf("unmarshal error: %w", err)
		}

		return nil
	})

	return eg.Wait()
}

func (bot *DiscoBot) guildCreate(s dg.Session, event *dg.GuildCreate) {
	voiceStates := event.Guild.VoiceStates
	for _, vs := range voiceStates {
		userID := vs.UserID
		bot.channelIDByUserID[userID] = vs.ChannelID
	}

	var commands = []*dg.CreateApplicationCommand{
		{Name: "disco", Description: "play music", Options: []*dg.ApplicationCommandOption{
			{
				Type:        dg.OptionTypeString,
				Name:        "url",
				Description: "YouTube video URL",
				Required:    true,
			},
		}},
		{Name: "disco-play", Description: "unpause"},
		{Name: "disco-pause", Description: "pause"},
		{Name: "disco-skip", Description: "skip the current track"},
		{Name: "disco-clean", Description: "clean the play queue"},
	}

	for i := range commands {
		if err := bot.client.ApplicationCommand(0).Guild(event.Guild.ID).Create(commands[i]); err != nil {
			log.Fatal(err)
		}
	}
}

func (bot *DiscoBot) handleInteractionCreate(s dg.Session, i *dg.InteractionCreate) {
	var err error

	switch i.Data.Name {
	case "disco":
		err = bot.handleDisco(s, i)
	case "disco-play":
		err = bot.handlePlay(s, i)
	case "disco-pause":
		err = bot.handlePause(s, i)
	case "disco-skip":
		err = bot.handleSkip(s, i)
	case "disco-clean":
		err = bot.handleClean(s, i)
	}

	if err != nil {
		log.Println(err)
	}
}

func (bot *DiscoBot) handleDisco(s dg.Session, i *dg.InteractionCreate) error {
	if i.Type != dg.InteractionApplicationCommand {
		return nil
	}

	url := i.Data.Options[0].Value.(string)

	channelID := bot.channelIDByUserID[i.Member.UserID]
	if err := bot.queueTrack(context.Background(), i.GuildID, channelID, url); err != nil {
		_ = s.SendInteractionResponse(context.Background(), i, &dg.CreateInteractionResponse{
			Type: dg.InteractionCallbackChannelMessageWithSource,
			Data: &dg.CreateInteractionResponseData{Content: err.Error()},
		})
		return fmt.Errorf("error playing sound: %w", err)
	}

	if err := s.SendInteractionResponse(context.Background(), i, &dg.CreateInteractionResponse{
		Type: dg.InteractionCallbackChannelMessageWithSource,
		Data: &dg.CreateInteractionResponseData{Content: fmt.Sprintf("Added %s to the play queue", url)},
	}); err != nil {
		return err
	}

	return nil
}

func (bot *DiscoBot) handlePause(s dg.Session, i *dg.InteractionCreate) error {
	if i.Type != dg.InteractionApplicationCommand {
		return nil
	}

	bot.playback.Pause()

	return s.SendInteractionResponse(context.Background(), i, &dg.CreateInteractionResponse{
		Type: dg.InteractionCallbackChannelMessageWithSource,
		Data: &dg.CreateInteractionResponseData{Content: "Paused..."},
	})
}

func (bot *DiscoBot) handlePlay(s dg.Session, i *dg.InteractionCreate) error {
	if i.Type != dg.InteractionApplicationCommand {
		return nil
	}

	bot.playback.Play()

	return s.SendInteractionResponse(context.Background(), i, &dg.CreateInteractionResponse{
		Type: dg.InteractionCallbackChannelMessageWithSource,
		Data: &dg.CreateInteractionResponseData{Content: "Playing..."},
	})
}

func (bot *DiscoBot) handleSkip(s dg.Session, i *dg.InteractionCreate) error {
	if i.Type != dg.InteractionApplicationCommand {
		return nil
	}

	bot.playback.Skip()

	return s.SendInteractionResponse(context.Background(), i, &dg.CreateInteractionResponse{
		Type: dg.InteractionCallbackChannelMessageWithSource,
		Data: &dg.CreateInteractionResponseData{Content: "Skip the current track"},
	})
}

func (bot *DiscoBot) handleClean(s dg.Session, i *dg.InteractionCreate) error {
	if i.Type != dg.InteractionApplicationCommand {
		return nil
	}

	bot.playQueue.Clean()
	bot.playback.Skip()

	return s.SendInteractionResponse(context.Background(), i, &dg.CreateInteractionResponse{
		Type: dg.InteractionCallbackChannelMessageWithSource,
		Data: &dg.CreateInteractionResponseData{Content: "Clean the play queue"},
	})
}
