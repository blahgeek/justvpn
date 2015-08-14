/*
* @Author: BlahGeek
* @Date:   2015-06-24
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-08-14
 */

package wire

import "net"
import "fmt"
import log "github.com/Sirupsen/logrus"

const UDP_DEFAULT_MTU = 1450

type UDPTransport struct {
	udp         *net.UDPConn
	remote_addr *net.UDPAddr
	is_server   bool
	mtu         int

	logger *log.Entry
}

func (trans *UDPTransport) MTU() int {
	return trans.mtu
}

func (trans *UDPTransport) Open(is_server bool, options map[string]interface{}) error {
	var server_addr, client_addr *net.UDPAddr
	var err error

	trans.logger = log.WithField("logger", "UDPTransport")

	if field := options["mtu"]; field == nil {
		trans.mtu = UDP_DEFAULT_MTU
	} else {
		trans.mtu = int(field.(float64))
	}

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
		trans.logger.WithField("addr", server_addr).Info("Listening on address")
		trans.udp, err = net.ListenUDP("udp", server_addr)
		if err != nil {
			return fmt.Errorf("Error listening UDP: %v", err)
		}
	} else {
		trans.logger.WithFields(log.Fields{
			"server": server_addr,
			"local":  client_addr,
		}).Info("Dialing to address")
		trans.udp, err = net.DialUDP("udp", client_addr, server_addr)
		if err != nil {
			return fmt.Errorf("Error dialing UDP: %v", err)
		}
		trans.remote_addr = server_addr
	}

	return nil
}

func (trans *UDPTransport) GetGateways() []net.IPNet {
	if trans.is_server {
		return make([]net.IPNet, 0)
	} else {
		return []net.IPNet{
			net.IPNet{
				trans.remote_addr.IP,
				net.IPv4Mask(255, 255, 255, 255),
			},
		}
	}
}

func (trans *UDPTransport) Close() error {
	if trans.udp == nil {
		return nil
	}
	return trans.udp.Close()
}

func (trans *UDPTransport) Read(buf []byte) (int, error) {
	rdlen, addr, err := trans.udp.ReadFromUDP(buf)
	if trans.is_server && err == nil {
		trans.remote_addr = addr
	}

	return rdlen, err
}

func (trans *UDPTransport) Write(buf []byte) (int, error) {
	if trans.remote_addr == nil && trans.is_server {
		return 0, fmt.Errorf("Remote UDP Address not available")
	}

	if trans.is_server {
		return trans.udp.WriteToUDP(buf, trans.remote_addr)
	} else {
		return trans.udp.Write(buf)
	}

	return 0, nil
}
