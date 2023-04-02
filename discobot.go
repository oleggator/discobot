package main

import (
	"bufio"
	"context"
	"fmt"
	dg "github.com/andersfylling/disgord"
	"github.com/at-wat/ebml-go"
	"github.com/at-wat/ebml-go/webm"
	"github.com/wader/goutubedl"
	"golang.org/x/exp/slices"
	"golang.org/x/sync/errgroup"
	"log"
)

func init() {
	goutubedl.Path = "yt-dlp"
}

type DiscoBot struct {
	client    *dg.Client
	playback  Playback
	playQueue Queue[*Task]
}

type Task struct {
	video              *goutubedl.Result
	guildID, channelID dg.Snowflake
}

func NewDiscoBot(token string) *DiscoBot {
	client := dg.New(dg.Config{
		BotToken: token,
		Intents:  dg.IntentGuilds | dg.IntentGuildMessages | dg.IntentGuildVoiceStates,
	})

	bot := &DiscoBot{
		client:    client,
		playback:  NewPlayback(),
		playQueue: NewQueue[*Task](32),
	}

	gateway := client.Gateway()
	gateway.GuildCreate(bot.guildCreate)
	gateway.InteractionCreate(bot.handleInteractionCreate)
	gateway.BotReady(func() {
		log.Println("bot is ready")
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
	video, err := goutubedl.New(ctx, url, goutubedl.Options{
		Type:              goutubedl.TypeSingle,
		DownloadThumbnail: false,
		DownloadSubtitles: false,
	})
	if err != nil {
		return err
	}

	if err := bot.playQueue.Push(&Task{
		video:     &video,
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
		reader, err := task.video.Download(ctx, "251")
		if err != nil {
			return err
		}
		defer reader.Close()

		var container Container
		container.Segment.ClustersChan = clusterChan

		err = ebml.Unmarshal(bufio.NewReader(reader), &container)
		close(clusterChan)
		if err != nil {
			return fmt.Errorf("unmarshal error: %w", err)
		}

		return nil
	})

	return eg.Wait()
}

func (bot *DiscoBot) guildCreate(s dg.Session, event *dg.GuildCreate) {
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
	go func() {
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
	}()
}

func (bot *DiscoBot) handleDisco(s dg.Session, i *dg.InteractionCreate) error {
	if i.Type != dg.InteractionApplicationCommand {
		return nil
	}

	url := i.Data.Options[0].Value.(string)

	ch, err := s.Channel(i.ChannelID).Get()
	if err != nil {
		return err
	}

	guild, err := s.Guild(ch.GuildID).Get()
	if err != nil {
		return err
	}

	// Look for the message sender in that guild's current voice states.
	vsIndex := slices.IndexFunc(guild.VoiceStates, func(vs *dg.VoiceState) bool {
		return vs.UserID == i.Member.UserID
	})
	if vsIndex == -1 {
		return fmt.Errorf("voice channel not found")
	}

	err = bot.queueTrack(context.Background(), guild.ID, guild.VoiceStates[vsIndex].ChannelID, url)
	if err != nil {
		_ = s.SendInteractionResponse(context.Background(), i, &dg.CreateInteractionResponse{
			Type: dg.InteractionCallbackChannelMessageWithSource,
			Data: &dg.CreateInteractionResponseData{Content: err.Error()},
		})
		return fmt.Errorf("error playing sound: %w", err)
	}

	if err = s.SendInteractionResponse(context.Background(), i, &dg.CreateInteractionResponse{
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
