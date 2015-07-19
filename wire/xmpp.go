/*
* @Author: BlahGeek
* @Date:   2015-07-02
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-07-19
 */

package wire

import "log"
import "net"
import "fmt"
import "strings"
import "crypto/tls"
import "encoding/base64"
import "github.com/mattn/go-xmpp"

const XMPP_DEFAULT_MTU = 1000

type XMPPTransport struct {
	client    *xmpp.Client
	remote_id string
	encoder   *base64.Encoding
	mtu       int
}

func (x *XMPPTransport) Open(is_server bool, options map[string]interface{}) error {
	xmpp.DefaultConfig = tls.Config{
		InsecureSkipVerify: true,
	}
	x.encoder = base64.StdEncoding

	if field := options["mtu"]; field == nil {
		x.mtu = XMPP_DEFAULT_MTU
	} else {
		x.mtu = int(field.(float64))
	}

	host := "talk.renren.com:5222"
	if opt_host := options["host"]; opt_host != nil {
		host = opt_host.(string)
	}

	var err error
	fetch_opt := func(server bool, field string) string {
		if err != nil {
			return ""
		}
		var key string
		if server {
			key = "server_" + field
		} else {
			key = "client_" + field
		}
		if field := options[key]; field != nil {
			return field.(string)
		}
		err = fmt.Errorf("`%s` not found in options", key)
		return ""
	}

	username := fetch_opt(is_server, "username")
	passwd := fetch_opt(is_server, "password")
	x.remote_id = fetch_opt(!is_server, "username")

	if err != nil {
		return err
	}

	xmpp_opts := xmpp.Options{
		Host:     host,
		User:     username,
		Password: passwd,
		NoTLS:    true,
		Debug:    false,
	}
	log.Printf("XMPP: Connecting to %s as %s, remote is %s\n",
		host, username, x.remote_id)
	x.client, err = xmpp_opts.NewClient()
	if err != nil {
		return err
	}

	return nil
}

func (x *XMPPTransport) MTU() int {
	return x.mtu
}

func (x *XMPPTransport) GetGateways() []net.IPNet {
	return []net.IPNet{} // FIXME
}

func (x *XMPPTransport) Close() error {
	return x.client.Close()
}

func (x *XMPPTransport) Write(buf []byte) (int, error) {
	str := x.encoder.EncodeToString(buf)
	msg := xmpp.Chat{
		Remote: x.remote_id,
		Type:   "chat",
		Text:   str,
	}
	_, err := x.client.Send(msg)
	return len(buf), err
}

func (x *XMPPTransport) Read(buf []byte) (int, error) {
	for {
		var msg interface{}
		var err error
		msg, err = x.client.Recv()
		switch chat := msg.(type) {
		case xmpp.Chat:
			if chat.Remote != x.remote_id {
				log.Printf("XMPP: Remote ID does not match: %v\n", chat.Remote)
				continue
			}
			if chat.Type == "chat_retry" {
				continue // FIXME
			}
			if len(chat.Text) == 0 {
				log.Println("XMPP: Empty text")
				continue
			}
			var dec_buf []byte
			dec_buf, err = x.encoder.DecodeString(chat.Text)
			if err != nil {
				if strings.Contains(chat.Text, "过于频繁") {
					log.Printf("XMPP server complains about too much messages\n")
				}
				log.Printf("XMPP: Unable to decode: %v\n", chat.Text)
				continue
			}
			return copy(buf, dec_buf), nil
		default:
		}
	}
	return 0, nil
}
