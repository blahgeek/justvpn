/*
* @Author: BlahGeek
* @Date:   2015-06-24
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-08-14
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
const VPN_ERROR_COUNT_THRESHOLD = 32

type VPN struct {
	from_tun, to_tun, from_wire, to_wire chan []byte

	wire_transports []wire.Transport
	wire_min_mtu    int

	tun_trans tun.Tun
	tun_mtu   int

	obfusecators []obfs.Obfusecator

	max_packet_cap int

	is_server bool
	wire_opts []interface{}
	obfs_opts []interface{}
	tun_opt   map[string]interface{}
}

func (vpn *VPN) initObfusecators() error {
	vpn.obfusecators = make([]obfs.Obfusecator, len(vpn.obfs_opts))
	vpn.tun_mtu = vpn.wire_min_mtu
	for index, opt_raw := range vpn.obfs_opts {
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

func (vpn *VPN) initWireTransport() error {
	vpn.wire_transports = make([]wire.Transport, len(vpn.wire_opts))
	vpn.wire_min_mtu = -1
	for index, opt_raw := range vpn.wire_opts {
		opt := opt_raw.(map[string]interface{})
		name := opt["name"].(string)
		options := opt["options"].(map[string]interface{})
		var err error
		vpn.wire_transports[index], err = wire.New(name, vpn.is_server, options)
		if err != nil {
			return err
		}
		mtu := vpn.wire_transports[index].MTU()
		if vpn.wire_min_mtu == -1 || mtu < vpn.wire_min_mtu {
			vpn.wire_min_mtu = mtu
		}
	}
	log.Printf("MTU for wire transport is %d\n", vpn.wire_min_mtu)
	return nil
}

func (vpn *VPN) initTunTransport() error {
	var err error
	vpn.tun_trans, err = tun.New()
	if err != nil {
		return err
	}

	tun_server_addr := net.ParseIP(vpn.tun_opt["server"].(string))
	tun_client_addr := net.ParseIP(vpn.tun_opt["client"].(string))
	if vpn.is_server {
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

	return tun.ApplyInterfaceRouter(vpn.tun_trans)
}

func (vpn *VPN) initRouter() error {
	wire_gw, err := tun.GetWireDefaultGateway()
	if err != nil {
		return err
	}
	log.Printf("Default gateway: %s\n", wire_gw.String())
	var wire_gateways []net.IPNet
	for _, wire_trans := range vpn.wire_transports {
		wire_gateways = append(wire_gateways, wire_trans.GetGateways()...)
	}
	return tun.ApplyRouter(wire_gateways, nil, wire_gw, net.IP{}, false)
}

func (vpn *VPN) Init(is_server bool, options map[string]interface{}) error {
	vpn.is_server = is_server
	vpn.wire_opts = options["wires"].([]interface{})
	vpn.obfs_opts = options["obfs"].([]interface{})
	vpn.tun_opt = options["tunnel"].(map[string]interface{})

	if err := vpn.initWireTransport(); err != nil {
		return err
	}
	if err := vpn.initObfusecators(); err != nil {
		return err
	}
	if err := vpn.initTunTransport(); err != nil {
		return err
	}
	if err := vpn.initRouter(); err != nil {
		return err
	}

	vpn.max_packet_cap = vpn.wire_min_mtu
	if vpn.tun_mtu > vpn.wire_min_mtu {
		vpn.max_packet_cap = vpn.tun_mtu
	}
	for _, obfusecator := range vpn.obfusecators {
		if max_packet := obfusecator.GetMaxPlainLength(); max_packet > vpn.max_packet_cap {
			vpn.max_packet_cap = max_packet
		}
	}
	log.Printf("Max packet capacity: %d\n", vpn.max_packet_cap)

	log.Printf("VPN Init done: %v wires <-> %d obfs <-> %v\n",
		len(vpn.wire_transports), len(vpn.obfusecators), vpn.tun_trans)

	return nil
}

func (vpn *VPN) readToChannel(reader io.Reader, mtu int, c chan<- []byte) {

	defer func() {
		log.Printf("Exit reading from %v", reader)
	}()

	var error_count int = 0
	for {
		buf := make([]byte, mtu, vpn.max_packet_cap)
		if rdlen, err := reader.Read(buf); rdlen == 0 || err != nil {
			log.Printf("Error reading from %v: %v, error count = %v",
				reader, err, error_count)
			if error_count > VPN_ERROR_COUNT_THRESHOLD {
				break
			}
		} else {
			error_count = 0
			c <- buf[:rdlen]
		}
	}
}

func (vpn *VPN) writeFromChannel(writer io.Writer, c <-chan []byte) {

	defer func() {
		log.Printf("Exit writing to %v", writer)
	}()

	var error_count int = 0
	for {
		buf, ok := <-c
		if !ok {
			log.Fatalf("Error reading from channel %v", c)
			break
		}
		if wlen, err := writer.Write(buf); wlen != len(buf) || err != nil {
			error_count += 1
			log.Printf("Error writing to %v: %v, error count = %v",
				writer, err, error_count)
			if error_count > VPN_ERROR_COUNT_THRESHOLD {
				break
			}
		} else {
			error_count = 0
		}
	}
}

func (vpn *VPN) obfsEncode(plain_c <-chan []byte, obfsed_c chan<- []byte) {

	defer func() {
		log.Printf("Exit obfs encoding")
	}()

	buffer := make([]byte, 0, vpn.max_packet_cap)
	for {
		data, ok := <-plain_c
		if !ok {
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

	defer func() {
		log.Printf("Exit obfs decoding")
	}()

	buffer := make([]byte, 0, vpn.max_packet_cap)
OuterLoop:
	for {
		data, ok := <-obfsed_c
		if !ok {
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

	vpn.from_wire = make(chan []byte, VPN_CHANNEL_BUFFER)
	vpn.to_wire = make(chan []byte, VPN_CHANNEL_BUFFER)
	for _, wire_trans := range vpn.wire_transports {
		go vpn.readToChannel(wire_trans, vpn.wire_min_mtu, vpn.from_wire)
		go vpn.writeFromChannel(wire_trans, vpn.to_wire)
	}

	vpn.from_tun = make(chan []byte, VPN_CHANNEL_BUFFER)
	vpn.to_tun = make(chan []byte, VPN_CHANNEL_BUFFER)
	go vpn.readToChannel(vpn.tun_trans, vpn.tun_mtu, vpn.from_tun)
	go vpn.writeFromChannel(vpn.tun_trans, vpn.to_tun)

	go vpn.obfsEncode(vpn.from_tun, vpn.to_wire)
	go vpn.obfsDecode(vpn.from_wire, vpn.to_tun)
}
