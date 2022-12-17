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
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/kkdai/youtube/v2"
	"github.com/schollz/progressbar/v3"

	"discobot/webm"
)

var token = os.Getenv("TOKEN")

func main() {
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatalln(err)
	}
	defer dg.Close()

	dg.AddHandler(guildCreate)
	dg.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsGuildVoiceStates

	dg.AddHandler(handleInteractionCreate)

	err = dg.Open()
	if err != nil {
		log.Fatalln(err)
	}

	fmt.Println("Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc
}

// playSound plays the current buffer to the provided channel.
func playSound(s *discordgo.Session, guildID, channelID, url string) error {
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
		case packet, ok := <-r.Chan:
			if !ok {
				log.Println("chan closed")
				break loop
			}

			vc.OpusSend <- packet.Data
			_ = bar.Add(len(packet.Data))
		case <-timeout.C:
			log.Println("timeout")
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

func guildCreate(s *discordgo.Session, event *discordgo.GuildCreate) {
	if event.Guild.Unavailable {
		return
	}

	appID := s.State.User.ID
	guildID := event.Guild.ID

	cmds, err := s.ApplicationCommandBulkOverwrite(appID, guildID, commands)
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

func handleInteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if handler, ok := commandHandlers[i.ApplicationCommandData().Name]; ok {
		handler(s, i)
	}
}
