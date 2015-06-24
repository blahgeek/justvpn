/*
* @Author: BlahGeek
* @Date:   2015-06-24
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-06-24
 */

package justvpn

import "log"
import "io"
import "github.com/blahgeek/justvpn/tun"
import "github.com/blahgeek/justvpn/wire"

func readToChannel(reader io.Reader, mtu int, c chan<- []byte) {
	for {
		buf := make([]byte, mtu)
		if rdlen, err := reader.Read(buf); rdlen == 0 || err != nil {
			log.Fatalf("Error reading from %v: %v", reader, err)
			break
		} else {
			c <- buf[:rdlen]
		}
	}
}

func writeFromChannel(writer io.Writer, c <-chan []byte) {
	for {
		buf, ok := <-c
		if !ok {
			log.Fatalf("Error reading from channel %v", c)
			break
		}
		if wlen, err := writer.Write(buf); wlen != len(buf) || err != nil {
			log.Fatalf("Error writing to %v: %v", writer, err)
			break
		}
	}
}

func StartRoute(t tun.Tun, w wire.Transport) error {
	tun_to_wire := make(chan []byte, 100) // TODO
	wire_to_tun := make(chan []byte, 100) // TODO

	mtu := w.MTU()
	log.Printf("Setting MTU to %v", mtu)
	if err := t.SetMTU(mtu); err != nil {
		return err
	}

	go readToChannel(t, mtu, tun_to_wire)
	go writeFromChannel(w, tun_to_wire)

	go readToChannel(w, mtu, wire_to_tun)
	go writeFromChannel(t, wire_to_tun)

	return nil

}
