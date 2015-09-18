/*
* @Author: BlahGeek
* @Date:   2015-08-29
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-09-18
 */

package main

import "fmt"
import "flag"
import "net"
import "github.com/miekg/dns"
import "github.com/blahgeek/justvpn/wire"

func main() {
	domain := flag.String("d", "x.blax.me", "Base domain")
	port := flag.Int("p", 53530, "Listen port")
	flag.Parse()

	fmt.Printf("Serving for %v at port %v\n", *domain, *port)

	udp_conn, err := net.ListenUDP("udp",
		&net.UDPAddr{IP: net.IPv4(0, 0, 0, 0), Port: *port})
	if err != nil {
		fmt.Printf("Unable to listen on %v: %v\n", *port, err)
		return
	}
	conn := wire.DNSUDPConn{udp_conn}

	for {
		query, addr, e := conn.ReadDNSFromUDP()
		if e != nil {
			fmt.Printf("Read query error: %v\n", err)
			continue
		}
		fmt.Printf("Query from %v: %v\n", addr, query)

		if len(query.Question) != 1 || query.Question[0].Qtype != dns.TypeTXT {
			fmt.Printf("Invalid query\n")
			continue
		}

		txt := new(dns.TXT)
		txt.Hdr = dns.RR_Header{Name: query.Question[0].Name, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 0}
		txt.Txt = []string{query.String()}
		reply := new(dns.Msg)
		reply.SetReply(query)
		reply.Answer = append(reply.Answer, txt)

		e = conn.WriteDNSToUDP(reply, addr)
		if e != nil {
			fmt.Printf("Write reply error: %v\n", e)
			continue
		}

	}
}
