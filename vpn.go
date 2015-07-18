/*
* @Author: BlahGeek
* @Date:   2015-06-24
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-07-18
 */

package justvpn

import "log"
import "io"
import "fmt"
import "net"
import "github.com/blahgeek/justvpn/tun"
import "github.com/blahgeek/justvpn/wire"
import "github.com/blahgeek/justvpn/obfs"

const VPN_CHANNEL_BUFFER = 64

type VPN struct {
	from_tun, to_tun, from_wire, to_wire chan []byte

	wire_trans wire.Transport
	wire_mtu   int

	tun_trans tun.Tun
	tun_mtu   int

	obfusecators []obfs.Obfusecator

	max_packet_cap int
}

func (vpn *VPN) initObfusecators(option_list []interface{}) error {
	vpn.obfusecators = make([]obfs.Obfusecator, len(option_list))
	vpn.tun_mtu = vpn.wire_mtu
	for index, opt_raw := range option_list {
		opt := opt_raw.(map[string]interface{})
		name := opt["name"].(string)
		options := opt["options"].(map[string]interface{})
		var err error
		vpn.obfusecators[index], err = obfs.New(name, options, vpn.tun_mtu)
		if err != nil {
			return fmt.Errorf("Error while allocating obfusecator %v: %v", name, err)
		}
		obfs_max_plain_len := vpn.obfusecators[index].GetMaxPlainLength()
		log.Printf("Obfs %s: MTU %d->%d\n", name, vpn.tun_mtu, obfs_max_plain_len)
		vpn.tun_mtu = obfs_max_plain_len
	}
	return nil
}

func (vpn *VPN) initWireTransport(is_server bool, opt map[string]interface{}) error {
	name := opt["name"].(string)
	options := opt["options"].(map[string]interface{})
	var err error
	vpn.wire_trans, err = wire.New(name, is_server, options)
	if err != nil {
		return err
	}

	vpn.wire_mtu = vpn.wire_trans.MTU()
	log.Printf("MTU for wire transport is %d\n", vpn.wire_mtu)
	return nil
}

func (vpn *VPN) initTunTransport(is_server bool, opt map[string]interface{}) error {
	var err error
	vpn.tun_trans, err = tun.New()
	if err != nil {
		return err
	}

	tun_server_addr := net.ParseIP(opt["server"].(string))
	tun_client_addr := net.ParseIP(opt["client"].(string))
	if is_server {
		log.Printf("Setting TUN IP: %v -> %v\n", tun_server_addr, tun_client_addr)
		vpn.tun_trans.SetIPv4(tun.ADDRESS, tun_server_addr)
		vpn.tun_trans.SetIPv4(tun.DST_ADDRESS, tun_client_addr)
	} else {
		log.Printf("Setting TUN IP: %v -> %v\n", tun_client_addr, tun_server_addr)
		vpn.tun_trans.SetIPv4(tun.ADDRESS, tun_client_addr)
		vpn.tun_trans.SetIPv4(tun.DST_ADDRESS, tun_server_addr)
	}

	log.Printf("MTU for TUN transport is %d\n", vpn.tun_mtu)
	vpn.tun_trans.SetMTU(vpn.tun_mtu)

	return nil
}

func (vpn *VPN) Init(is_server bool, options map[string]interface{}) error {
	wire_opt := options["wire"].(map[string]interface{})
	if err := vpn.initWireTransport(is_server, wire_opt); err != nil {
		return err
	}
	obfs_list := options["obfs"].([]interface{})
	if err := vpn.initObfusecators(obfs_list); err != nil {
		return err
	}
	tun_opt := options["tunnel"].(map[string]interface{})
	if err := vpn.initTunTransport(is_server, tun_opt); err != nil {
		return err
	}

	vpn.max_packet_cap = vpn.wire_mtu
	if vpn.tun_mtu > vpn.wire_mtu {
		vpn.max_packet_cap = vpn.tun_mtu
	}
	for _, obfusecator := range vpn.obfusecators {
		if max_packet := obfusecator.GetMaxPlainLength(); max_packet > vpn.max_packet_cap {
			vpn.max_packet_cap = max_packet
		}
	}
	log.Printf("Max packet capacity: %d\n", vpn.max_packet_cap)

	log.Printf("VPN Init done: %v <-> %d obfs <-> %v\n",
		vpn.wire_trans, len(vpn.obfusecators), vpn.tun_trans)

	return nil
}

func (vpn *VPN) readToChannel(reader io.Reader, mtu int, c chan<- []byte) {
	for {
		buf := make([]byte, mtu, vpn.max_packet_cap)
		if rdlen, err := reader.Read(buf); rdlen == 0 || err != nil {
			log.Fatalf("Error reading from %v: %v", reader, err)
			break
		} else {
			c <- buf[:rdlen]
		}
	}
}

func (vpn *VPN) writeFromChannel(writer io.Writer, c <-chan []byte) {
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

func (vpn *VPN) obfsEncode(plain_c <-chan []byte, obfsed_c chan<- []byte) {
	buffer := make([]byte, 0, vpn.max_packet_cap)
	for {
		data, ok := <-plain_c
		if !ok {
			log.Fatalf("Error reading from channel %v", plain_c)
			break
		}
		for _, obfusecator := range vpn.obfusecators {
			dst := buffer[:cap(buffer)]
			enclen := obfusecator.Encode(data, dst)
			data, buffer = dst[:enclen], data
		}
		obfsed_c <- data
	}
}

func (vpn *VPN) obfsDecode(obfsed_c <-chan []byte, plain_c chan<- []byte) {
	buffer := make([]byte, 0, vpn.max_packet_cap)
OuterLoop:
	for {
		data, ok := <-obfsed_c
		if !ok {
			log.Fatalf("Error reading from channel %v", obfsed_c)
			break
		}
		// reverse
		for i := len(vpn.obfusecators) - 1; i >= 0; i-- {
			dst := buffer[:cap(buffer)]
			if declen, err := vpn.obfusecators[i].Decode(data, dst); err != nil {
				log.Printf("Error while decoding by %v, drop\n", vpn.obfusecators[i])
				continue OuterLoop
			} else {
				data, buffer = dst[:declen], data
			}
		}
		plain_c <- data
	}
}

func (vpn *VPN) Start() {
	vpn.from_tun = make(chan []byte, VPN_CHANNEL_BUFFER)
	vpn.to_tun = make(chan []byte, VPN_CHANNEL_BUFFER)
	vpn.from_wire = make(chan []byte, VPN_CHANNEL_BUFFER)
	vpn.to_wire = make(chan []byte, VPN_CHANNEL_BUFFER)

	go vpn.readToChannel(vpn.tun_trans, vpn.tun_mtu, vpn.from_tun)
	go vpn.readToChannel(vpn.wire_trans, vpn.wire_mtu, vpn.from_wire)
	go vpn.writeFromChannel(vpn.tun_trans, vpn.to_tun)
	go vpn.writeFromChannel(vpn.wire_trans, vpn.to_wire)

	go vpn.obfsEncode(vpn.from_tun, vpn.to_wire)
	go vpn.obfsDecode(vpn.from_wire, vpn.to_tun)
}
