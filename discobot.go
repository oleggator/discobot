package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"discobot/bufferedreadseeker"

	"github.com/bwmarrin/discordgo"
	"github.com/ebml-go/webm"
	"github.com/kkdai/youtube/v2"
	"github.com/schollz/progressbar/v3"
)

type DiscoBot struct {
	api           *discordgo.Session
	PlayStatus    bool
	StartPlayback chan struct{}
}

func NewDiscoBot(token string) (*DiscoBot, error) {
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, err
	}

	bot := &DiscoBot{
		api:           dg,
		StartPlayback: make(chan struct{}),
	}

	dg.AddHandler(bot.guildCreate)
	dg.AddHandler(bot.handleInteractionCreate)
	dg.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsGuildVoiceStates

	return bot, nil
}

func (bot *DiscoBot) Open() error {
	return bot.api.Open()
}

func (bot *DiscoBot) Close() error {
	return bot.api.Close()
}

// playSound plays the current buffer to the provided channel.
func (bot *DiscoBot) playSound(s *discordgo.Session, guildID, channelID, url string) error {
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

	reader, n, err := client.GetStreamContext(context.Background(), video, format)
	if err != nil {
		return err
	}

	bar := progressbar.DefaultBytes(n)
	barReader := progressbar.NewReader(reader, bar)
	bufReader := bufferedreadseeker.NewReaderWithSize(&barReader, int(n))

	// Join the provided voice channel.
	vc, err := s.ChannelVoiceJoin(guildID, channelID, false, true)
	if err != nil {
		return err
	}

	// Sleep for a specified amount of time before playing the sound
	time.Sleep(250 * time.Millisecond)

	// Start speaking.
	vc.Speaking(true)

	r, err := webm.Parse(bufReader, &webm.WebM{})
	if err != nil {
		return err
	}
	for packet := range r.Chan {
		if packet.Timecode == webm.BadTC {
			r.Shutdown()
		} else {
			if bot.PlayStatus {
				vc.OpusSend <- packet.Data
			} else {
				<-bot.StartPlayback
			}
		}
	}

	// Stop speaking
	vc.Speaking(false)

	// Sleep for a specificed amount of time before ending.
	time.Sleep(250 * time.Millisecond)

	// Disconnect from the provided voice channel.
	vc.Disconnect()

	return nil
}

func (bot *DiscoBot) guildCreate(s *discordgo.Session, event *discordgo.GuildCreate) {
	if event.Guild.Unavailable {
		return
	}

	appID := s.State.User.ID
	guildID := event.Guild.ID

	cmds, err := s.ApplicationCommandBulkOverwrite(appID, guildID, []*discordgo.ApplicationCommand{
		{
			Name:        "disco",
			Description: "Showcase of single autocomplete option",
			Type:        discordgo.ChatApplicationCommand,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:         "url",
					Description:  "YouTube video URL",
					Type:         discordgo.ApplicationCommandOptionString,
					Required:     true,
					Autocomplete: true,
				},
			},
		},
		{
			Name:        "disco-play",
			Description: "Pause current song",
			Type:        discordgo.ChatApplicationCommand,
		},
		{
			Name:        "disco-pause",
			Description: "Pause current song",
			Type:        discordgo.ChatApplicationCommand,
		},
	})
	if err != nil {
		log.Println(err)
	}

	s.AddHandlerOnce(func(s *discordgo.Session, event *discordgo.Disconnect) {
		for _, cmd := range cmds {
			err := s.ApplicationCommandDelete(appID, guildID, cmd.ID)
			if err != nil {
				log.Fatalf("Cannot delete %q command: %v", cmd.Name, err)
			}
		}
	})
}

func (bot *DiscoBot) handleInteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	var err error

	switch i.ApplicationCommandData().Name {
	case "disco":
		err = bot.handleDisco(s, i)
	case "disco-play":
		err = bot.handlePlay(s, i)
	case "disco-pause":
		err = bot.handlePause(s, i)
	}

	if err != nil {
		log.Println(err)
	}
}

func (bot *DiscoBot) handleDisco(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		// Find the channel that the message came from.
		c, err := s.State.Channel(i.ChannelID)
		if err != nil {
			// Could not find channel.
			return err
		}

		data := i.ApplicationCommandData()
		url := data.Options[0].StringValue()

		err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: fmt.Sprintf("Playing %q...", data.Options[0].StringValue())},
		})
		if err != nil {
			return err
		}

		// Find the guild for that channel.
		g, err := s.State.Guild(c.GuildID)
		if err != nil {
			// Could not find guild.
			return err
		}

		// Look for the message sender in that guild's current voice states.
		for _, vs := range g.VoiceStates {
			if vs.UserID == i.Member.User.ID {
				bot.PlayStatus = true
				err = bot.playSound(s, g.ID, vs.ChannelID, url)
				if err != nil {
					return fmt.Errorf("error playing sound: %w", err)
				}

				return nil
			}
		}
	case discordgo.InteractionApplicationCommandAutocomplete:
		return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionApplicationCommandAutocompleteResult,
			Data: &discordgo.InteractionResponseData{Choices: []*discordgo.ApplicationCommandOptionChoice{
				{Name: "Rick Astley", Value: "https://www.youtube.com/watch?v=dQw4w9WgXcQ"},
				{Name: "Short video", Value: "https://www.youtube.com/watch?v=LQxwqsoxXQI"},
			}},
		})
	}

	return nil
}

func (bot *DiscoBot) handlePause(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	if i.Type != discordgo.InteractionApplicationCommand {
		return nil
	}

	bot.PlayStatus = false

	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: "Paused..."},
	})
}

func (bot *DiscoBot) handlePlay(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	if i.Type != discordgo.InteractionApplicationCommand {
		return nil
	}

	bot.PlayStatus = true
	//bot.StartPlayback <- struct{}{}
	select {
	case bot.StartPlayback <- struct{}{}:
	default:
		// skip if nobody is waiting
	}

	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: fmt.Sprintf("Playing...")},
	})
}
