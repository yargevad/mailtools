package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/yargevad/mailtools/imaputil"
)

var mbox = flag.String("mbox", "INBOX", "mailbox name")
var newer = flag.String("newer", "", "message received date must be more recent")
var subject = flag.String("subject", "", "message must contain substring in subject")
var download = flag.Bool("download", false, "should matching messages be downloaded")

func main() {
	flag.Parse()
	hasSubject := (subject != nil && *subject != "")
	hasNewer := (newer != nil && *newer != "")
	hasDownload := (download != nil && *download != false)

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	ctx := imaputil.ImapCtx{}

	ctx.Host = os.Getenv("CLIMAP_HOST")
	if ctx.Host == "" {
		log.Fatal("No IMAP host set in environment! (CLIMAP_HOST)\n")
	}

	ctx.User = os.Getenv("CLIMAP_USER")
	if ctx.User == "" {
		log.Fatal("No IMAP user set in environment! (CLIMAP_USER)\n")
	}

	ctx.Pass = os.Getenv("CLIMAP_PASS")
	if ctx.Pass == "" {
		log.Fatal("No IMAP password set in environment! (CLIMAP_PASS)\n")
	}

	baseDir := os.Getenv("CLIMAP_BASE")
	if baseDir == "" && hasDownload {
		log.Fatal("No base directory set for saving messages! (CLIMAP_BASE)\n")
	}

	serverName := os.Getenv("CLIMAP_TLS_SERVERNAME")
	if serverName != "" {
		ctx.TLS.ServerName = serverName
	}

	err := ctx.Init()
	if err != nil {
		if strings.HasPrefix(err.Error(), "x509: certificate is valid for ") {
			log.Printf("HINT: set CLIMAP_TLS_SERVERNAME to work around certificate domain mismatches\n")
		}
		log.Fatal(err)
	}
	defer ctx.IMAP.Logout(10 * time.Second)

	log.Printf("Login successful for %s at %s\n", ctx.User, ctx.Host)

	if !hasSubject && !hasNewer {
		os.Exit(0)
	}

	err = ctx.Mailbox(*mbox)
	if err != nil {
		log.Fatal(err)
	}

	// Make sure there's a mailbox directory
	msgDir := fmt.Sprintf("%s/%s", baseDir, ctx.IMAP.Mailbox.Name)
	if hasDownload {
		err = os.Mkdir(msgDir, 0755)
		if err != nil {
			if !os.IsExist(err) {
				log.Fatal(err)
			}
		}
	}

	var crit []string
	if hasNewer {
		dur, err := time.ParseDuration(*newer)
		if err != nil {
			log.Fatal(err)
		}
		sinceStr := time.Now().Add(-dur).Format("2-Jan-2006")
		sinceStr, ok := ctx.IMAP.Quote(sinceStr).(string)
		if !ok {
			log.Fatalf("Error quoting date [%s]\n", sinceStr)
		}
		crit = append(crit, []string{"SINCE", sinceStr}...)
	}

	if hasSubject {
		subjectStr, ok := ctx.IMAP.Quote(*subject).(string)
		if !ok {
			log.Fatalf("Error quoting subject [%s]\n", subjectStr)
		}
		crit = append(crit, []string{"SUBJECT", subjectStr}...)
	}

	uids, err := ctx.Search(crit)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("search returned %d elements:\n", len(uids))
	for idx, uid := range uids {
		log.Printf("- uid=%d (%d/%d)\n", uid, idx, len(uids))
		if hasDownload {
			msgFile := fmt.Sprintf("%s/%d.eml", msgDir, uid)
			file, err := os.Open(msgFile)
			if err == nil {
				defer file.Close()
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
				log.Printf("  got %d bytes for uid %d\n", len(msgBytes), uid)
			} else {
				log.Fatal(err)
			}
		}
	}
}
