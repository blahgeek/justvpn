/*
* @Author: BlahGeek
* @Date:   2015-06-24
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-08-13
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
const VPN_RESTART_ERROR_THRESHOLD = 10

type VPN struct {
	from_tun, to_tun, from_wire, to_wire chan []byte

	wire_trans wire.Transport
	wire_mtu   int

	tun_trans tun.Tun
	tun_mtu   int

	obfusecators []obfs.Obfusecator

	max_packet_cap int

	is_server bool
	wire_opt  map[string]interface{}
	obfs_opts []interface{}
	tun_opt   map[string]interface{}
}

func (vpn *VPN) initObfusecators() error {
	vpn.obfusecators = make([]obfs.Obfusecator, len(vpn.obfs_opts))
	vpn.tun_mtu = vpn.wire_mtu
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
	name := vpn.wire_opt["name"].(string)
	options := vpn.wire_opt["options"].(map[string]interface{})
	var err error
	vpn.wire_trans, err = wire.New(name, vpn.is_server, options)
	if err != nil {
		return err
	}

	vpn.wire_mtu = vpn.wire_trans.MTU()
	log.Printf("MTU for wire transport is %d\n", vpn.wire_mtu)
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
	if err := tun.ApplyInterfaceRouter(vpn.tun_trans); err != nil {
		return err
	}
	wire_gw, err := tun.GetWireDefaultGateway()
	if err != nil {
		return err
	}
	log.Printf("Default gateway: %s\n", wire_gw.String())
	return tun.ApplyRouter(vpn.wire_trans.GetGateways(), nil,
		wire_gw, net.IP{}, false)
}

func (vpn *VPN) Init(is_server bool, options map[string]interface{}) error {
	vpn.is_server = is_server
	vpn.wire_opt = options["wire"].(map[string]interface{})
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

func (vpn *VPN) readToChannel(reader io.Reader, mtu int,
	c chan<- []byte, exit_chan chan<- int) {

	defer func() {
		log.Printf("Exit reading from %v", reader)
		exit_chan <- 0
	}()

	var error_count int = 0
	for {
		buf := make([]byte, mtu, vpn.max_packet_cap)
		if rdlen, err := reader.Read(buf); rdlen == 0 || err != nil {
			log.Printf("Error reading from %v: %v, error count = %v",
				reader, err, error_count)
			if error_count > VPN_RESTART_ERROR_THRESHOLD {
				break
			}
		} else {
			error_count = 0
			c <- buf[:rdlen]
		}
	}
}

func (vpn *VPN) writeFromChannel(writer io.Writer,
	c <-chan []byte, exit_chan chan<- int) {

	defer func() {
		log.Printf("Exit writing to %v", writer)
		exit_chan <- 0
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
			if error_count > VPN_RESTART_ERROR_THRESHOLD {
				break
			}
		} else {
			error_count = 0
		}
	}
}

func (vpn *VPN) obfsEncode(plain_c <-chan []byte, obfsed_c chan<- []byte,
	exit_chan chan int) {

	defer func() {
		log.Printf("Exit obfs encoding")
		exit_chan <- 0
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

func (vpn *VPN) obfsDecode(obfsed_c <-chan []byte, plain_c chan<- []byte,
	exit_chan chan int) {

	defer func() {
		log.Printf("Exit obfs decoding")
		exit_chan <- 0
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

	tun_exit := make(chan int)
	wire_exit := make(chan int)
	obfs_exit := make(chan int)

	start_wire := func() {
		vpn.from_wire = make(chan []byte, VPN_CHANNEL_BUFFER)
		vpn.to_wire = make(chan []byte, VPN_CHANNEL_BUFFER)
		go vpn.readToChannel(vpn.wire_trans, vpn.wire_mtu, vpn.from_wire, wire_exit)
		go vpn.writeFromChannel(vpn.wire_trans, vpn.to_wire, wire_exit)
	}

	start_tun := func() {
		vpn.from_tun = make(chan []byte, VPN_CHANNEL_BUFFER)
		vpn.to_tun = make(chan []byte, VPN_CHANNEL_BUFFER)
		go vpn.readToChannel(vpn.tun_trans, vpn.tun_mtu, vpn.from_tun, tun_exit)
		go vpn.writeFromChannel(vpn.tun_trans, vpn.to_tun, tun_exit)
	}

	start_obfs := func() {
		go vpn.obfsEncode(vpn.from_tun, vpn.to_wire, obfs_exit)
		go vpn.obfsDecode(vpn.from_wire, vpn.to_tun, obfs_exit)
	}

	start_wire()
	start_tun()
	start_obfs()

	go func() {
		select {
		case _ = <-wire_exit:
			log.Printf("Error detected in wire transport")
			close(vpn.from_wire)
			close(vpn.to_wire)
			_ = <-wire_exit // wait for another goroutine to exit
			_ = <-obfs_exit
			_ = <-obfs_exit
			log.Printf("Restarting wire transport")
			vpn.wire_trans.Close()
			if err := vpn.initWireTransport(); err != nil {
				log.Fatal(err)
			}
			start_wire()
			start_obfs()
		case _ = <-tun_exit:
			log.Printf("Error detected in tun transport")
			close(vpn.from_tun)
			close(vpn.to_tun)
			_ = <-tun_exit // wait for another goroutine to exit
			_ = <-obfs_exit
			_ = <-obfs_exit
			log.Printf("Restarting tun transport")
			vpn.tun_trans.Destroy()
			if err := vpn.initTunTransport(); err != nil {
				log.Fatal(err)
			}
			if err := vpn.initRouter(); err != nil {
				log.Fatal(err)
			}
			start_tun()
			start_obfs()
		}
	}()
}
