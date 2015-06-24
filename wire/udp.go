/*
* @Author: BlahGeek
* @Date:   2015-06-24
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-06-24
 */

package wire

import "net"
import "fmt"
import "log"

type UDPTransport struct {
	udp         *net.UDPConn
	remote_addr *net.UDPAddr
	is_server   bool
}

func (trans *UDPTransport) MTU() int {
	return 1460 // TODO
}

func (trans *UDPTransport) Open(is_server bool, options map[string]interface{}) error {
	var server_addr, client_addr *net.UDPAddr
	var err error

	trans.is_server = is_server

	if field := options["server_addr"]; field == nil {
		return fmt.Errorf("`server_addr` not found in options")
	} else {
		server_addr, err = net.ResolveUDPAddr("udp", field.(string))
		if err != nil {
			return fmt.Errorf("Error resolving server addr: %v", err)
		}
	}

	if field := options["client_addr"]; field != nil {
		client_addr, err = net.ResolveUDPAddr("udp", field.(string))
		if err != nil {
			return fmt.Errorf("Error resolving client addr: %v", err)
		}
	}

	if is_server {
		log.Printf("UDP Server: listening on %v", server_addr)
		trans.udp, err = net.ListenUDP("udp", server_addr)
		if err != nil {
			return fmt.Errorf("Error listening UDP: %v", err)
		}
	} else {
		log.Printf("UDP Client: dialing to %v from %v", server_addr, client_addr)
		trans.udp, err = net.DialUDP("udp", client_addr, server_addr)
		if err != nil {
			return fmt.Errorf("Error dialing UDP: %v", err)
		}
	}

	return nil
}

func (trans *UDPTransport) Close() error {
	if trans.udp == nil {
		return nil
	}
	return trans.udp.Close()
}

func (trans *UDPTransport) Read(buf []byte) (int, error) {
	if trans.udp == nil {
		return 0, fmt.Errorf("UDP Transport not available")
	}
	rdlen, addr, err := trans.udp.ReadFromUDP(buf)
	if trans.is_server && err == nil && trans.remote_addr == nil {
		log.Printf("UDP Server: read from %v", addr)
		trans.remote_addr = addr
	}
	return rdlen, err
}

func (trans *UDPTransport) Write(buf []byte) (int, error) {
	if trans.udp == nil {
		return 0, fmt.Errorf("UDP Transport not available")
	}
	if trans.remote_addr == nil && trans.is_server {
		return 0, fmt.Errorf("Remote UDP Address not available")
	}

	if trans.is_server {
		return trans.udp.WriteToUDP(buf, trans.remote_addr)
	} else {
		return trans.udp.Write(buf)
	}
}
