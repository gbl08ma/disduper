package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gbl08ma/disduper/bot"
	"github.com/gbl08ma/keybox"
)

var mainLog = log.New(os.Stdout, "", log.Ldate|log.Ltime)

func main() {
	mainLog.Println("Disduper starting, opening keybox...")
	secrets, err := keybox.Open(SecretsPath)
	if err != nil {
		mainLog.Fatalln(err)
	}
	mainLog.Println("Keybox opened")

	token, present := secrets.Get("discordToken")
	if !present {
		mainLog.Fatalln("Discord token not present in keybox")
	}

	bot := new(bot.Disduper)
	err = bot.Start(token, mainLog)
	if err != nil {
		mainLog.Fatalln("Bot failed to start: " + err.Error())
	}
	// Wait here until CTRL-C or other term signal is received.
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// Cleanly close down the Discord session.
	bot.Stop()
}
