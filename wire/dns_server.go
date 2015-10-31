/*
* @Author: BlahGeek
* @Date:   2015-09-13
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-10-31
 */

package wire

import "net"
import "time"
import "encoding/json"
import "github.com/miekg/dns"
import log "github.com/Sirupsen/logrus"

type DNSTransportServerOptions struct {
	BaseDomain string `json:"base_domain"`
	Port       int    `json:"port"`
}

const DNSSERVER_QUERY_BUFSIZE = 10240
const DNSSERVER_QUERY_TIMEOUT = 3 * time.Second

type DNSServerQuery struct {
	Msg  *dns.Msg
	Addr *net.UDPAddr
	Time time.Time
}

type DNSTransportServer struct {
	conn    *DNSUDPConn
	options DNSTransportServerOptions
	logger  *log.Entry

	queries       chan DNSServerQuery
	UpstreamBuf   chan []byte
	DownstreamBuf chan []byte

	upstream_codec   *DNSTransportStream
	downstream_codec *DNSTransportStream
}

func (trans *DNSTransportServer) Open(options json.RawMessage) error {
	trans.logger = log.WithField("logger", "DNSTransportServer")
	if err := json.Unmarshal(options, &trans.options); err != nil {
		return err
	} else {
		if trans.options.Port == 0 {
			trans.options.Port = 53
		}
		trans.logger.WithFields(log.Fields{
			"domain": trans.options.BaseDomain,
			"port":   trans.options.Port,
		}).Info("Starting new DNS Server")
	}

	if udp_conn, err := net.ListenUDP("udp", &net.UDPAddr{
		IP:   net.IPv4(0, 0, 0, 0),
		Port: trans.options.Port,
	}); err != nil {
		return err
	} else {
		trans.conn = &DNSUDPConn{udp_conn}
	}

	trans.queries = make(chan DNSServerQuery, DNSSERVER_QUERY_BUFSIZE)
	trans.UpstreamBuf = make(chan []byte)
	trans.DownstreamBuf = make(chan []byte)

	var err error
	trans.upstream_codec = &DNSTransportStream{}
	if trans.upstream_codec.codec, err = NewDNSTransportUpstreamCodec(trans.options.BaseDomain); err != nil {
		return err
	}
	trans.downstream_codec = &DNSTransportStream{codec: NewDNSTransportDownstreamCodec()}

	// read dns query, decode it, put it into channel
	go func() {
		for {
			msg, addr, err := trans.conn.ReadDNSFromUDP()
			if err != nil {
				trans.logger.WithField("error", err).Warn("Error reading DNS query")
				continue
			} else {
				if msg.Response || len(msg.Question) == 0 || msg.Question[0].Qtype != dns.TypeTXT {
					trans.logger.WithField("msg", msg).Warn("Unknown DNS query")
					continue
				}
				trans.queries <- DNSServerQuery{
					Msg:  msg,
					Addr: addr,
					Time: time.Now(),
				}
				decoded_msg := trans.upstream_codec.Decode(msg.Question[0].Name)
				if decoded_msg != nil {
					trans.UpstreamBuf <- decoded_msg
				}
			}
		}
	}()

	// read from channel, encode it, send it
	go func() {
		for {
			data := <-trans.DownstreamBuf
			encoded_msgs := trans.downstream_codec.Encode(data)

			for _, msg := range encoded_msgs {
				now := time.Now()
				var query DNSServerQuery
				for {
					query = <-trans.queries
					if query.Time.Add(DNSSERVER_QUERY_TIMEOUT).After(now) {
						break
					}
				}
				txt := new(dns.TXT)
				txt.Hdr = dns.RR_Header{Name: query.Msg.Question[0].Name, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 0}
				txt.Txt = []string{msg}
				reply := new(dns.Msg)
				reply.SetReply(query.Msg)
				reply.Answer = append(reply.Answer, txt)

				err := trans.conn.WriteDNSToUDP(reply, query.Addr)
				if err != nil {
					trans.logger.WithField("error", err).Warn("Error writing to UDP")
					continue
				}
			}
		}
	}()

	return nil
}
