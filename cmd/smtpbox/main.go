package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/peterbourgon/ff/v3"
	"github.com/yavosh/smtpbox/http"
	"github.com/yavosh/smtpbox/inmem"
	"github.com/yavosh/smtpbox/smtp"
)

var (
	httpPort   int
	smtpPort   int
	smtpDomain string
)

func main() {
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	fs.IntVar(&httpPort, "http-port", 8080, "listen port for the http server")
	fs.IntVar(&smtpPort, "smtp-port", 1025, "listen port for the smtp server")
	fs.StringVar(&smtpDomain, "smtp-domain", "localhost", "domain for smtp server")
	err := ff.Parse(fs, os.Args[1:], ff.WithEnvVarNoPrefix())
	if err != nil {
		log.Fatalf("flag set: %v", err)
	}

	l := log.New(os.Stdout, "smtp ", log.LstdFlags)

	backend := inmem.NewEmailService()

	smtpServer := smtp.NewServer(
		fmt.Sprintf(":%d", smtpPort),
		smtpDomain,
		backend,
	)

	httpServer := http.NewServer(
		httpPort,
		backend,
	)

	if err := httpServer.Start(); err != nil {
		l.Fatalf("error starting http %v", err)
	}

	smtpServer.Start()

	//if err := smtpServer.Start(); err != nil {
	//	l.Fatalf("error starting smtp %v", err)
	//}

	// Setting up signal capturing
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, os.Kill, syscall.SIGINT, syscall.SIGTERM)

	// Waiting for SIGINT (pkill -2)
	<-stop

	if err := smtpServer.Stop(); err != nil {
		l.Fatal(err)
	}

	if err := httpServer.Stop(); err != nil {
		l.Fatal(err)
	}
}
