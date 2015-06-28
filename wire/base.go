/*
* @Author: BlahGeek
* @Date:   2015-06-24
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-06-24
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

	switch name {
	case "udp":
		log.Printf("New wire transport: UDPTransport")
		ret = &UDPTransport{}
		err := ret.Open(is_server, options)
		return ret, err
	default:
	}
	return ret, fmt.Errorf("No wire transport found: %v", name)
}