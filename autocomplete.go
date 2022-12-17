package main

import (
	"fmt"
	"github.com/bwmarrin/discordgo"
	"log"
)

var (
	commands = []*discordgo.ApplicationCommand{
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
	}

	commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"disco": func(s *discordgo.Session, i *discordgo.InteractionCreate) {

			switch i.Type {
			case discordgo.InteractionApplicationCommand:
				// Find the channel that the message came from.
				c, err := s.State.Channel(i.ChannelID)
				if err != nil {
					// Could not find channel.
					return
				}

				data := i.ApplicationCommandData()
				url := data.Options[0].StringValue()

				err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{Content: fmt.Sprintf("Playing %q...", data.Options[0].StringValue())},
				})
				if err != nil {
					log.Println(err)
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
					if vs.UserID == i.Member.User.ID {
						err = playSound(s, g.ID, vs.ChannelID, url)
						if err != nil {
							fmt.Println("Error playing sound:", err)
						}

						return
					}
				}
			case discordgo.InteractionApplicationCommandAutocomplete:
				err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionApplicationCommandAutocompleteResult,
					Data: &discordgo.InteractionResponseData{Choices: []*discordgo.ApplicationCommandOptionChoice{
						{Name: "Rick Astley", Value: "https://www.youtube.com/watch?v=dQw4w9WgXcQ"},
						{Name: "Short video", Value: "https://www.youtube.com/watch?v=LQxwqsoxXQI"},
					}},
				})
				if err != nil {
					log.Println(err)
					return
				}
			}
		},
	}
)
