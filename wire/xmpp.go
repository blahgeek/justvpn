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
import "encoding/json"
import "github.com/mattn/go-xmpp"
import log "github.com/Sirupsen/logrus"

const XMPP_DEFAULT_MTU = 1000

type XMPPTransportOptions struct {
	MTU            float64 `json:"mtu"`
	Host           string  `json:"host"`
	ServerUsername string  `json:"server_username"`
	ServerPassword string  `json:"server_password"`
	ClientUsername string  `json:"client_username"`
	ClientPassword string  `json:"client_password"`
}

type XMPPTransport struct {
	client    *xmpp.Client
	remote_id string
	encoder   *base64.Encoding
	mtu       int

	logger *log.Entry
	opt    XMPPTransportOptions
}

func (x *XMPPTransport) String() string {
	return fmt.Sprintf("XMPP[%v]", x.remote_id)
}

func (x *XMPPTransport) Open(is_server bool, options json.RawMessage) error {
	var err error
	x.logger = log.WithField("logger", "XMPPTransport")

	if err = json.Unmarshal(options, &x.opt); err != nil {
		return err
	}

	xmpp.DefaultConfig = tls.Config{
		InsecureSkipVerify: true,
	}
	x.encoder = base64.StdEncoding

	x.mtu = XMPP_DEFAULT_MTU
	if x.opt.MTU > 0 {
		x.mtu = int(x.opt.MTU)
	}

	if len(x.opt.Host) == 0 {
		x.opt.Host = "talk.renren.com:5222"
	}

	username := x.opt.ClientUsername
	passwd := x.opt.ClientPassword
	x.remote_id = x.opt.ServerUsername
	if is_server {
		username = x.opt.ServerUsername
		passwd = x.opt.ServerPassword
		x.remote_id = x.opt.ClientUsername
	}

	xmpp_opts := xmpp.Options{
		Host:     x.opt.Host,
		User:     username,
		Password: passwd,
		NoTLS:    true,
		Debug:    false,
	}
	x.logger.WithFields(log.Fields{
		"server":    fmt.Sprintf("%s@%s", username, x.opt.Host),
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
	hostname := strings.Split(x.opt.Host, ":")[0]
	x.logger.WithField("hostname", hostname).
		Debug("Looking up IP Address for XMPP Host")
	ips, err := net.LookupIP(hostname)
	if err != nil {
		x.logger.WithField("error", err).
			Warning("Error looking up IP Address for XMPP Host")
		return []net.IPNet{}
	}
	var ret []net.IPNet
	for _, ip := range ips {
		mask_len := len(ip) * 8
		ret = append(ret, net.IPNet{ip, net.CIDRMask(mask_len, mask_len)})
	}
	return ret
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
