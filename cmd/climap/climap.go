package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime"
	"mime/multipart"
	"net/mail"
	"os"
	"strings"
	"time"

	humanize "github.com/dustin/go-humanize"
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
	msgDir := fmt.Sprintf("%s/%s/%s", baseDir, ctx.User, ctx.IMAP.Mailbox.Name)
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
		var msgBytes []byte
		if hasDownload {
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
				// parse top-level message
				buf := bytes.NewReader(msgBytes)
				m, err := mail.ReadMessage(buf)
				if err != nil {
					log.Fatal(err)
				}

				// get Content-Type, pull out boundary
				ctype := m.Header.Get("Content-Type")
				// TODO: skip if not multipart/mixed at top level
				mtype, params, err := mime.ParseMediaType(ctype)
				if err != nil {
					log.Fatal(err)
				}
				log.Printf("Content-Type: %s\n", mtype)
				// TODO: skip if no "boundary" key
				for k, v := range params {
					log.Printf("%s = %s\n", k, v)
				}

				buf.Seek(0, 0)
				mpr := multipart.NewReader(buf, params["boundary"])

				for {
					// content-disposition: attachment
					part, err := mpr.NextPart()
					if err != nil {
						if err == io.EOF {
							break
						}
						log.Fatal(err)
					}
					if attFile := part.FileName(); attFile != "" {
						log.Printf("found attachment: %s\n", attFile)
						attBytes := make([]byte, 5*1024*1024)
						idx := uint64(0)
						for {
							n, err := part.Read(attBytes[idx:])
							if err != nil {
								if err == io.EOF {
									break
								}
								log.Fatal(err)
							}
							idx += uint64(n)
							//log.Printf("read %d bytes to %d\n", n, idx)
						}
						log.Printf("read %s from %s\n", humanize.Bytes(idx), attFile)
						// TODO: do "stuff" with in-memory zip file
					}

				}
			}

		}
	}
}
