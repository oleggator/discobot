package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/kkdai/youtube/v2"
	"github.com/schollz/progressbar/v3"

	"discobot/webm"
)

const nggyu = "https://www.youtube.com/watch?v=dQw4w9WgXcQ"

var token = os.Getenv("TOKEN")

func main() {
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatalln(err)
	}
	defer dg.Close()

	dg.AddHandler(messageCreate)
	dg.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsGuildVoiceStates

	err = dg.Open()
	if err != nil {
		log.Fatalln(err)
	}

	fmt.Println("Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc
}

// This function will be called (due to AddHandler above) every time a new
// message is created on any channel that the autenticated bot has access to.
func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore all messages created by the bot itself
	// This isn't required in this specific example, but it's a good practice.
	if m.Author.ID == s.State.User.ID {
		return
	}

	// check if the message is "!airhorn"
	if strings.HasPrefix(m.Content, "!rickroll") {
		// Find the channel that the message came from.
		c, err := s.State.Channel(m.ChannelID)
		if err != nil {
			// Could not find channel.
			return
		}

		// Find the guild for that channel.
		g, err := s.State.Guild(c.GuildID)
		if err != nil {
			// Could not find guild.
			return
		}

		// Look for the message sender in that guild's current voice states.
		for _, vs := range g.VoiceStates {
			if vs.UserID == m.Author.ID {
				err = playSound(s, g.ID, vs.ChannelID)
				if err != nil {
					fmt.Println("Error playing sound:", err)
				}

				return
			}
		}
	}
}

// playSound plays the current buffer to the provided channel.
func playSound(s *discordgo.Session, guildID, channelID string) error {
	client := youtube.Client{
		Debug:      false,
		HTTPClient: http.DefaultClient,
	}

	video, err := client.GetVideoContext(context.Background(), nggyu)
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

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, &barReader); err != nil {
		return err
	}

	// Join the provided voice channel.
	vc, err := s.ChannelVoiceJoin(guildID, channelID, false, true)
	if err != nil {
		return err
	}

	// Sleep for a specified amount of time before playing the sound
	time.Sleep(250 * time.Millisecond)

	// Start speaking.
	vc.Speaking(true)

	var w webm.WebM
	r, err := webm.Parse(bytes.NewReader(buf.Bytes()), &w)
	if err != nil {
		return err
	}

	bar = progressbar.NewOptions(-1, progressbar.OptionShowBytes(true))
	defer bar.Close()
loop:
	for {
		timeout := time.NewTimer(5 * time.Second)
		select {
		case packet := <-r.Chan:
			vc.OpusSend <- packet.Data
			_ = bar.Add(len(packet.Data))
		case <-timeout.C:
			break loop
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
