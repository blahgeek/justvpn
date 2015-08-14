/*
* @Author: BlahGeek
* @Date:   2015-06-24
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-08-14
 */

package wire

import "io"
import log "github.com/Sirupsen/logrus"
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
	log.WithField("name", name).Info("Allocating new wire transport")

	switch name {
	case "udp":
		ret = &UDPTransport{}
	case "xmpp":
		ret = &XMPPTransport{}
	default:
		return ret, fmt.Errorf("No wire transport found: %v", name)
	}

	err := ret.Open(is_server, options)
	return ret, err
}
