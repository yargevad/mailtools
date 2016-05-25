package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"time"

	humanize "github.com/dustin/go-humanize"
	"github.com/yargevad/mailtools/imaputil"
	"github.com/yargevad/mailtools/mimeutil"
)

var mbox = flag.String("mbox", "INBOX", "mailbox name")
var newer = flag.String("newer", "", "message received date must be more recent")
var subject = flag.String("subject", "", "message must contain substring in subject")
var download = flag.Bool("download", false, "should matching messages be downloaded")

func main() {
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lshortfile)

	ctx, err := imaputil.EnvConnect("CLIMAP_")
	if err != nil {
		log.Fatal(err)
	}

	baseDir := os.Getenv("CLIMAP_BASE")
	if baseDir == "" && *download == true {
		log.Fatal("No base directory set for saving messages! (CLIMAP_BASE)\n")
	}

	defer ctx.IMAP.Logout(10 * time.Second)

	log.Printf("Login successful for %s at %s\n", ctx.User, ctx.Host)

	if *subject == "" && *newer == "" {
		os.Exit(0)
	}

	err = ctx.Mailbox(*mbox)
	if err != nil {
		log.Fatal(err)
	}

	// Make sure there's a local mailbox directory
	msgDir := fmt.Sprintf("%s/%s/%s", baseDir, ctx.User, ctx.IMAP.Mailbox.Name)
	if *download == true {
		err = os.Mkdir(msgDir, 0755)
		if err != nil {
			if !os.IsExist(err) {
				log.Fatal(err)
			}
		}
	}

	var criteria []string
	if *newer != "" {
		dur, err := time.ParseDuration(*newer)
		if err != nil {
			log.Fatal(err)
		}
		sinceStr := time.Now().Add(-dur).Format("2-Jan-2006")
		sinceStr, ok := ctx.IMAP.Quote(sinceStr).(string)
		if !ok {
			log.Fatalf("Error quoting date [%s]\n", sinceStr)
		}
		criteria = append(criteria, "SINCE", sinceStr)
	}

	if *subject != "" {
		subjectStr, ok := ctx.IMAP.Quote(*subject).(string)
		if !ok {
			log.Fatalf("Error quoting subject [%s]\n", subjectStr)
		}
		criteria = append(criteria, "SUBJECT", subjectStr)
	}

	uids, err := ctx.Search(criteria)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("search returned %d elements:\n", len(uids))
	for idx, uid := range uids {
		log.Printf("- uid=%d (%d/%d)\n", uid, idx, len(uids))
		var msgBytes []byte
		if *download == true {
			msgFile := fmt.Sprintf("%s/%d.eml", msgDir, uid)
			file, err := os.Open(msgFile)
			if err == nil {
				defer file.Close()
				msgBytes, err = ioutil.ReadFile(msgFile)
				if err != nil {
					log.Fatal(err)
				}
				log.Printf("  file cached for uid %d: %s\n", uid, msgFile)
			} else if os.IsNotExist(err) {
				file, err := os.Create(msgFile)
				if err != nil {
					log.Fatal(err)
				}
				defer file.Close()

				msgBytes, err := ctx.MessageByUID(uid)
				n, err := file.Write(msgBytes)
				if err == nil && n < len(msgBytes) {
					err = io.ErrShortWrite
				}
				if err != nil {
					log.Fatal(err)
				}
				log.Printf("  saved %d bytes for uid %d\n", len(msgBytes), uid)

			} else {
				log.Fatal(err)
			}

			if msgBytes != nil {
				att, err := mimeutil.DecodeAttachment(msgBytes)
				if err != nil {
					log.Fatal(err)
				}
				log.Printf("read %s from %s\n", humanize.Bytes(att.Length), att.Filename)
			}

		}
	}
}
