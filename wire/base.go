/*
* @Author: BlahGeek
* @Date:   2015-06-24
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-06-28
 */

package wire

import "io"
import "log"
import "fmt"

type Transport interface {
	// For both server and client side
	Open(is_server bool, options map[string]interface{}) error
	Close() error

	// Max packet size the transport can write
	MTU() int

	io.Reader
	io.Writer
}

func New(name string, is_server bool, options map[string]interface{}) (Transport, error) {
	var ret Transport
	err := fmt.Errorf("No wire transport found: %v", name)

	switch name {
	case "udp":
		log.Printf("New wire transport: UDPTransport")
		ret = &UDPTransport{}
		err = ret.Open(is_server, options)
	default:
	}

	return ret, err
}
