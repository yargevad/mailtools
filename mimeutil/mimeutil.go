package mimeutil

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"unicode"
)

const (
	defaultBufSize = 4096
)

type Attachment struct {
	Filename string
	Length   uint64
	Content  []byte
	frags    [][]byte
	encoding string
}

func GenBoundary() ([]byte, error) {
	var buf [45]byte
	var enc [60]byte
	_, err := io.ReadFull(rand.Reader, buf[:])
	if err != nil {
		return nil, err
	}
	base64.StdEncoding.Encode(enc[:], buf[:])
	return enc[:], nil
}

// DecodeAttachment returns the content of the first attachment in a multipart MIME message.
func DecodeAttachment(msg []byte) (*Attachment, error) {
	msgBuf := bytes.NewReader(msg)
	m, err := mail.ReadMessage(msgBuf)
	if err != nil {
		return nil, err
	}

	ctype := m.Header.Get("Content-Type")
	mtype, params, err := mime.ParseMediaType(ctype)
	if err != nil {
		return nil, err
	}

	if !strings.HasPrefix(mtype, "multipart/") {
		return nil, fmt.Errorf("Unsupported top-level Content-Type [%s]", mtype)
	}

	if _, ok := params["boundary"]; !ok {
		return nil, fmt.Errorf("No boundary in Content-Type!")
	}

	msgBuf.Seek(0, 0)
	mpr := multipart.NewReader(msgBuf, params["boundary"])

	for {
		part, err := mpr.NextPart()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		// TODO: require that filenames match a pattern
		if attFile := part.FileName(); attFile != "" {
			var bufs [][]byte
			attLen := uint64(0)
			for {
				buf := make([]byte, 4*1024)
				n, err := part.Read(buf)
				if err != nil {
					if err == io.EOF {
						break
					}
					return nil, err
				}
				attLen += uint64(n)
				bufs = append(bufs, buf[:n])
			}
			att := &Attachment{Filename: attFile, Length: attLen, frags: bufs}
			att.encoding = part.Header.Get("Content-Transfer-Encoding")
			att.Content = bytes.Join(att.frags, []byte(""))
			if att.encoding == "" {
			} else if att.encoding == "base64" {
				// remove whitespace
				tmp := bytes.Map(func(r rune) rune {
					if unicode.IsSpace(r) {
						return -1
					}
					return r
				}, att.Content)
				n, err := base64.StdEncoding.Decode(att.Content, tmp)
				if err != nil {
					return nil, err
				}
				att.Content = att.Content[:n]
				att.Length = uint64(n)

			} else {
				return att, fmt.Errorf("Unsupported Content-Transfer-Encoding [%s]", att.encoding)
			}
			return att, nil
		}

	}
	return nil, nil
}
