/*
* @Author: BlahGeek
* @Date:   2015-07-02
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-08-14
 */

package wire

import "net"
import "fmt"
import "strings"
import "crypto/tls"
import "encoding/base64"
import "github.com/mattn/go-xmpp"
import log "github.com/Sirupsen/logrus"

const XMPP_DEFAULT_MTU = 1000

type XMPPTransport struct {
	client    *xmpp.Client
	remote_id string
	encoder   *base64.Encoding
	mtu       int

	logger *log.Entry
}

func (x *XMPPTransport) String() string {
	return fmt.Sprintf("XMPP[%v]", x.remote_id)
}

func (x *XMPPTransport) Open(is_server bool, options map[string]interface{}) error {
	x.logger = log.WithField("logger", "XMPPTransport")

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
	x.logger.WithFields(log.Fields{
		"server":    fmt.Sprintf("%s@%s", username, host),
		"remote_id": x.remote_id,
	}).Info("Connecting to remote")
	x.client, err = xmpp_opts.NewClient()
	if err != nil {
		return err
	}

	return nil
}

func (x *XMPPTransport) MTU() int {
	return x.mtu
}

func (x *XMPPTransport) GetWireNetworks() []net.IPNet {
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
				x.logger.WithField("remote_id", chat.Remote).
					Warning("Remote ID does not match")
				continue
			}
			if chat.Type == "chat_retry" {
				continue // FIXME
			}
			if len(chat.Text) == 0 {
				x.logger.Warning("Empty text")
				continue
			}
			var dec_buf []byte
			dec_buf, err = x.encoder.DecodeString(chat.Text)
			if err != nil {
				if strings.Contains(chat.Text, "过于频繁") {
					x.logger.Warning("Server complains about too much messages")
				} else {
					x.logger.WithField("text", chat.Text).Warning("Unable to decode")
				}
				continue
			}
			return copy(buf, dec_buf), nil
		default:
		}
	}
	return 0, nil
}
