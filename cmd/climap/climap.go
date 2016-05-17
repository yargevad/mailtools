package main

import (
	"crypto/tls"
	"log"
	"os"
	"strings"

	"github.com/mxk/go-imap/imap"
)

func main() {
	host := os.Getenv("CLIMAP_HOST")
	if host == "" {
		log.Fatal("No IMAP host set in environment! (CLIMAP_HOST)\n")
	}

	user := os.Getenv("CLIMAP_USER")
	if user == "" {
		log.Fatal("No IMAP user set in environment! (CLIMAP_USER)\n")
	}

	pass := os.Getenv("CLIMAP_PASS")
	if pass == "" {
		log.Fatal("No IMAP password set in environment! (CLIMAP_PASS)\n")
	}

	tlsConfig := &tls.Config{}
	serverName := os.Getenv("CLIMAP_TLS_SERVERNAME")
	if serverName != "" {
		tlsConfig.ServerName = serverName
	}

	_, err := imap.DialTLS(host, tlsConfig)
	if err != nil {
		if strings.HasPrefix(err.Error(), "x509: certificate is valid for ") {
			log.Printf("HINT: set CLIMAP_TLS_SERVERNAME to work around certificate domain mismatches\n")
		}
		log.Fatal(err)
	}

}
