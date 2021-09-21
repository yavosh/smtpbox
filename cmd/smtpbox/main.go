// Package main starts the smtpbox server
package main

import (
	"flag"
	"fmt"
	"github.com/yavosh/smtpbox/dns"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/peterbourgon/ff/v3"
	"github.com/yavosh/smtpbox/http"
	"github.com/yavosh/smtpbox/inmem"
	"github.com/yavosh/smtpbox/smtp"
)

func main() {
	var (
		httpPort   int
		smtpPort   int
		smtpDomain string
		dnsPort    int
		dnsDomain  string
	)

	fs := flag.NewFlagSet("server", flag.ExitOnError)
	fs.IntVar(&httpPort, "http-port", 8080, "listen port for the http server")
	fs.IntVar(&smtpPort, "smtp-port", 1025, "listen port for the smtp server")
	fs.StringVar(&smtpDomain, "smtp-domain", "localhost", "domain for smtp server")
	fs.IntVar(&dnsPort, "dns-port", 53, "listen port for the smtp server")
	fs.StringVar(&dnsDomain, "dns-domain", "localhost", "domain for smtp server")

	err := ff.Parse(fs, os.Args[1:], ff.WithEnvVarNoPrefix())
	if err != nil {
		log.Fatalf("flag set: %v", err)
	}

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

	dnsServer := dns.NewServer(
		dnsPort,
		dnsDomain,
	)

	if err := httpServer.Start(); err != nil {
		log.Fatalf("error starting http %v", err)
	}

	if err := dnsServer.Start(); err != nil {
		log.Fatalf("error starting dns %v", err)
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
		log.Fatal(err)
	}

	if err := httpServer.Stop(); err != nil {
		log.Fatal(err)
	}

	if err := dnsServer.Stop(); err != nil {
		log.Fatal(err)
	}
}
