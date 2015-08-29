/*
* @Author: BlahGeek
* @Date:   2015-08-29
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-08-29
 */

package main

import "fmt"
import "flag"
import "net"
import "github.com/blahgeek/justvpn/wire"

func main() {
	domain := flag.String("d", "x.blax.me", "Base domain")
	port := flag.Int("p", 53530, "Listen port")
	flag.Parse()

	fmt.Printf("Serving for %v at port %v\n", *domain, *port)

	conn, err := net.ListenUDP("udp",
		&net.UDPAddr{IP: net.IPv4(0, 0, 0, 0), Port: *port})
	if err != nil {
		fmt.Printf("Unable to listen on %v: %v\n", *port, err)
		return
	}

	var fac *wire.DNSPacketFactory
	fac, err = wire.NewDNSPacketFactory(*domain)
	if err != nil {
		fmt.Printf("New DNS Packet factory error: %v\n", err)
		return
	}

	for {
		var in_bytes [1500]byte
		var in_addr *net.UDPAddr
		if _, in_addr, err = conn.ReadFromUDP(in_bytes[:]); err != nil {
			fmt.Printf("Read from UDP error: %v\n", err)
			break
		}
		if id, data, e := fac.ParseDNSQuery(in_bytes[:]); e != nil {
			fmt.Printf("Parse DNS Query error: %v\n", e)
			break
		} else {
			fmt.Printf("Got query: %v, id = %v\n", string(data), id)
			result := fac.MakeDNSResult(id, data, 600, data)
			conn.WriteToUDP(result, in_addr)
		}
	}
}
