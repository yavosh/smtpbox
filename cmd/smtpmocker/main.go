package main

import (
	"github.com/yavosh/smtpmocker/agents/http"
	"github.com/yavosh/smtpmocker/agents/smtp"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {

	smtpServer := smtp.NewServer("localhost",
		smtp.WithListenAddr(":1025"),
	)

	httpServer := http.NewServer(http.WithListenAddr(":5000"))

	httpServer.Start()
	smtpServer.Start()

	// Setting up signal capturing
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, os.Kill, syscall.SIGINT, syscall.SIGTERM)

	// Waiting for SIGINT (pkill -2)
	<-stop

	if err := smtpServer.Stop(); err != nil {
		log.Fatal(err)
	}

	if err := httpServer.Stop(); err != nil {
		log.Fatal(err)
	}

}
