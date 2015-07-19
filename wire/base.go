/*
* @Author: BlahGeek
* @Date:   2015-06-24
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-07-18
 */

package wire

import "io"
import "log"
import "fmt"
import "net"

type Transport interface {
	// For both server and client side
	Open(is_server bool, options map[string]interface{}) error
	Close() error

	// Max packet size the transport can write
	MTU() int

	// Return IP Networks that should be routed via non-vpn gateway
	GetGateways() []net.IPNet

	io.Reader
	io.Writer
}

func New(name string, is_server bool, options map[string]interface{}) (Transport, error) {
	var ret Transport

	switch name {
	case "udp":
		log.Println("New wire transport: UDPTransport")
		ret = &UDPTransport{}
	case "xmpp":
		log.Println("New wire transport: XMPPTransport")
		ret = &XMPPTransport{}
	default:
		return ret, fmt.Errorf("No wire transport found: %v", name)
	}

	err := ret.Open(is_server, options)
	return ret, err
}
