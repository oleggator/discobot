package main

import (
	"bufio"
	"context"
	"fmt"
	"github.com/andersfylling/disgord"
	"github.com/at-wat/ebml-go"
	"github.com/at-wat/ebml-go/webm"
	"github.com/kkdai/youtube/v2"
	"log"
	"net/http"
	"sync"
)

type DiscoBot struct {
	client        *disgord.Client
	PlayStatus    bool
	StartPlayback chan struct{}
}

func NewDiscoBot(token string) (*DiscoBot, error) {
	client := disgord.New(disgord.Config{
		BotToken: token,
		Intents:  disgord.IntentGuilds | disgord.IntentGuildMessages | disgord.IntentGuildVoiceStates,
	})

	bot := &DiscoBot{
		client:        client,
		StartPlayback: make(chan struct{}),
	}

	gateway := client.Gateway()
	gateway.GuildCreate(bot.guildCreate)
	gateway.InteractionCreate(bot.handleInteractionCreate)
	gateway.BotReady(func() {
		log.Println("bot is ready")
	})

	return bot, nil
}

func (bot *DiscoBot) Open() error {
	return bot.client.Gateway().Connect()
}

func (bot *DiscoBot) Close() error {
	return bot.client.Gateway().Disconnect()
}

type Segment struct {
	SeekHead *webm.SeekHead    `ebml:"SeekHead"`
	Info     webm.Info         `ebml:"Info"`
	Tracks   webm.Tracks       `ebml:"Tracks"`
	Cues     *webm.Cues        `ebml:"Cues"`
	Cluster  chan webm.Cluster `ebml:"Cluster"`
}

type Container struct {
	Header  webm.EBMLHeader `ebml:"EBML"`
	Segment Segment         `ebml:"Segment"`
}

// playSound plays the current buffer to the provided channel.
func (bot *DiscoBot) playSound(guildID, channelID disgord.Snowflake, url string) error {
	client := youtube.Client{
		Debug:      false,
		HTTPClient: http.DefaultClient,
	}

	video, err := client.GetVideoContext(context.Background(), url)
	if err != nil {
		return err
	}
	formats := video.Formats.WithAudioChannels().Type("opus")
	//log.Println(formats)
	format := &formats[0]

	reader, _, err := client.GetStreamContext(context.Background(), video, format)
	if err != nil {
		return err
	}

	// Join the provided voice channel.
	voice, err := bot.client.Guild(guildID).VoiceChannel(channelID).Connect(false, true)
	if err != nil {
		return err
	}
	defer voice.Close()

	// Start speaking.
	voice.StartSpeaking()
	defer voice.StopSpeaking()

	// cluster's size is usually below about 175 000 bytes
	clusterChan := make(chan webm.Cluster, 32)

	var wg sync.WaitGroup
	wg.Add(1)

	go func(clusterChan <-chan webm.Cluster) {
		defer wg.Done()
		for cluster := range clusterChan {
			for _, block := range cluster.SimpleBlock {
				for _, data := range block.Data {
					if bot.PlayStatus {
						if err := voice.SendOpusFrame(data); err != nil {
							return
						}
					} else {
						<-bot.StartPlayback
					}
				}
			}
		}
	}(clusterChan)

	var container Container
	container.Segment.Cluster = clusterChan

	bufReader := bufio.NewReader(reader)
	err = ebml.Unmarshal(bufReader, &container)
	close(clusterChan)
	if err != nil {
		return fmt.Errorf("unmarshal error: %w", err)
	}

	wg.Wait()

	return nil
}

func (bot *DiscoBot) guildCreate(s disgord.Session, event *disgord.GuildCreate) {
	var commands = []*disgord.CreateApplicationCommand{
		{Name: "disco", Description: "play music", Options: []*disgord.ApplicationCommandOption{
			{
				Type:        disgord.OptionTypeString,
				Name:        "url",
				Description: "YouTube video URL",
				Required:    true,
			},
		}},
		{Name: "disco-play", Description: "unpause"},
		{Name: "disco-pause", Description: "pause"},
	}

	for i := range commands {
		if err := bot.client.ApplicationCommand(0).Guild(event.Guild.ID).Create(commands[i]); err != nil {
			log.Fatal(err)
		}
	}
}

func (bot *DiscoBot) handleInteractionCreate(s disgord.Session, i *disgord.InteractionCreate) {
	go func() {
		var err error

		switch i.Data.Name {
		case "disco":
			err = bot.handleDisco(s, i)
		case "unpause":
			err = bot.handlePlay(s, i)
		case "pause":
			err = bot.handlePause(s, i)
		}

		if err != nil {
			log.Println(err)
		}
	}()
}

func (bot *DiscoBot) handleDisco(s disgord.Session, i *disgord.InteractionCreate) error {
	if i.Type != disgord.InteractionApplicationCommand {
		return nil
	}

	urlArg := i.Data.Options[0]

	ch, err := s.Channel(i.ChannelID).Get()
	if err != nil {
		return err
	}

	guild, err := s.Guild(ch.GuildID).Get()
	if err != nil {
		return err
	}

	url := urlArg.Value.(string)

	if err = s.SendInteractionResponse(context.Background(), i, &disgord.CreateInteractionResponse{
		Type: disgord.InteractionCallbackChannelMessageWithSource,
		Data: &disgord.CreateInteractionResponseData{Content: fmt.Sprintf("Playing %s...", url)},
	}); err != nil {
		return err
	}

	for _, vs := range guild.VoiceStates {
		if vs.UserID == i.Member.UserID {
			bot.PlayStatus = true
			if err = bot.playSound(guild.ID, vs.ChannelID, url); err != nil {
				return err
			}
			return err
		}
	}

	return nil
}

func (bot *DiscoBot) handlePause(s disgord.Session, i *disgord.InteractionCreate) error {
	if i.Type != disgord.InteractionApplicationCommand {
		return nil
	}

	bot.PlayStatus = false

	return s.SendInteractionResponse(context.Background(), i, &disgord.CreateInteractionResponse{
		Type: disgord.InteractionCallbackChannelMessageWithSource,
		Data: &disgord.CreateInteractionResponseData{Content: "Paused..."},
	})
}

func (bot *DiscoBot) handlePlay(s disgord.Session, i *disgord.InteractionCreate) error {
	if i.Type != disgord.InteractionApplicationCommand {
		return nil
	}

	bot.PlayStatus = true
	//bot.StartPlayback <- struct{}{}
	select {
	case bot.StartPlayback <- struct{}{}:
	default:
		// skip if nobody is waiting
	}

	return s.SendInteractionResponse(context.Background(), i, &disgord.CreateInteractionResponse{
		Type: disgord.InteractionCallbackChannelMessageWithSource,
		Data: &disgord.CreateInteractionResponseData{Content: "Playing..."},
	})
}
