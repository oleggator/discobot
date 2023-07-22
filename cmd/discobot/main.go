package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"discobot"
)

var token = os.Getenv("TOKEN")

func main() {
	ctx, cancel := context.WithCancel(context.Background())

	bot := discobot.NewDiscoBot(token)
	if err := bot.Open(ctx); err != nil {
		log.Fatalln(err)
	}
	defer bot.Close()

	go func() {
		sc := make(chan os.Signal, 1)
		signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)

		fmt.Println("Press CTRL-C to exit.")

		<-sc
		cancel()
	}()

	if err := bot.RunPlayer(ctx); err != nil {
		log.Println(err)
	}
}
