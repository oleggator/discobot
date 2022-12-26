package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

var token = os.Getenv("TOKEN")

func main() {
	bot, err := NewDiscoBot(token)
	if err != nil {
		log.Fatalln(err)
	}
	if err := bot.Open(); err != nil {
		log.Fatalln(err)
	}
	defer bot.Close()

	go func() {
		if err := bot.RunPlayer(context.Background()); err != nil {
			log.Println(err)
		}
	}()

	fmt.Println("Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc
}
