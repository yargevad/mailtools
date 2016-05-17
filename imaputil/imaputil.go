package imaputil

import (
	"crypto/tls"
	"fmt"

	"github.com/mxk/go-imap/imap"
)

type ImapCtx struct {
	Host string
	User string
	Pass string
	TLS  tls.Config
	IMAP *imap.Client
}

// Connect reaches out to the server. It doesn't login, yet.
func (ctx *ImapCtx) Connect() error {
	c, err := imap.DialTLS(ctx.Host, &ctx.TLS)
	if err != nil {
		return err
	}
	ctx.IMAP = c
	return nil
}

// Ping runs a no-op to test the server connection.
func (ctx *ImapCtx) Ping() error {
	_, err := CheckOK(ctx.IMAP.Noop())
	if err != nil {
		return err
	}
	return nil
}

// Login authenticates with the server using the configured credentials.
func (ctx *ImapCtx) Login() (*imap.Command, error) {
	defer ctx.IMAP.SetLogMask(Sensitive(ctx.IMAP, "LOGIN"))
	return ctx.IMAP.Login(ctx.User, ctx.Pass)
}

// Init is a convenience method which calls Connect, Ping, then Login.
func (ctx *ImapCtx) Init() error {
	if ctx.IMAP == nil {
		err := ctx.Connect()
		if err != nil {
			return err
		}
	}
	err := ctx.Ping()
	if err != nil {
		return err
	}
	_, err = ctx.Login()
	if err != nil {
		return err
	}
	return nil
}

func (ctx *ImapCtx) Mailbox(boxName string) error {
	cmd, err := CheckOK(ctx.IMAP.List("", boxName))
	if err != nil {
		return err
	}
	if len(cmd.Data) == 0 {
		return fmt.Errorf("Mailbox [%s] not found", boxName)
	}
	for _, boxData := range cmd.Data {
		box := boxData.MailboxInfo()
		_, err := CheckOK(ctx.IMAP.Select(box.Name, true))
		if err != nil {
			return err
		}
	}
	return nil
}

func (ctx *ImapCtx) Search(terms []string) (uids []uint32, err error) {
	var fields []imap.Field
	for _, term := range terms {
		fields = append(fields, term)
	}
	search, err := CheckOK(ctx.IMAP.Search(fields))
	if err != nil {
		return nil, err
	}
	if search.Data != nil && len(search.Data) > 0 {
		for _, searchRes := range search.Data {
			uidRes := searchRes.SearchResults()
			for _, uid := range uidRes {
				uids = append(uids, uid)
			}
		}
	}
	return uids, nil
}

func (ctx *ImapCtx) MessageByUID(uid uint32) ([]byte, error) {
	set, err := imap.NewSeqSet(fmt.Sprintf("%d", uid))
	if err != nil {
		return nil, err
	}

	fetch, err := CheckOK(ctx.IMAP.Fetch(set, "BODY[]"))
	if err != nil {
		return nil, err
	}

	if len(fetch.Data) > 0 {
		return BodyFromFields(fetch.Data[0].Fields), nil
	}

	return nil, nil
}

func BodyFromFields(fields []imap.Field) []byte {
	for _, field := range fields {
		ftype := imap.TypeOf(field)
		if ftype == imap.List {
			fmap := imap.AsFieldMap(field)
			for k, v := range fmap {
				vtype := imap.TypeOf(v)
				if k == "BODY[]" && vtype == imap.LiteralString {
					return imap.AsBytes(v)
				}
			}
		}
	}
	return nil
}

func CheckOK(cmd *imap.Command, err error) (*imap.Command, error) {
	if cmd == nil {
		return nil, fmt.Errorf("Got nil imap.Command!")
	} else if err == nil {
		_, err = cmd.Result(imap.OK)
	}
	if err != nil {
		return nil, fmt.Errorf("IMAP %s error: %s", cmd.Name(true), err)
	}
	return cmd, nil
}

func Login(c *imap.Client, user, pass string) (cmd *imap.Command, err error) {
	defer c.SetLogMask(Sensitive(c, "LOGIN"))
	return c.Login(user, pass)
}

func Sensitive(c *imap.Client, action string) imap.LogMask {
	mask := c.SetLogMask(imap.LogConn)
	hide := imap.LogCmd | imap.LogRaw
	if mask&hide != 0 {
		c.Logln(imap.LogConn, "Raw logging disabled during", action)
	}
	c.SetLogMask(mask &^ hide)
	return mask
}
