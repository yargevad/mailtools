package mimeutil

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
)

const (
	defaultBufSize = 4096
)

type Attachment struct {
	Filename string
	Length   uint64
	content  []byte
	frags    [][]byte
}

func (a *Attachment) Content() []byte {
	if a.content != nil && a.frags == nil {
		return a.content
	} else if a.content == nil && a.frags != nil {
		a.content = bytes.Join(a.frags, []byte(""))
		a.frags = nil
		return a.content
	}
	return nil
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

	if mtype != "multipart/mixed" {
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
			return att, nil
		}

	}
	return nil, nil
}
