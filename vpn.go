/*
* @Author: BlahGeek
* @Date:   2015-06-24
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-06-28
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
}

func (vpn *VPN) initObfusecators(option_list []interface{}) error {
	vpn.obfusecators = make([]obfs.Obfusecator, len(option_list))
	for index, opt_raw := range option_list {
		opt := opt_raw.(map[string]interface{})
		name := opt["name"].(string)
		options := opt["options"].(map[string]interface{})
		var err error
		vpn.obfusecators[index], err = obfs.New(name, options)
		if err != nil {
			return fmt.Errorf("Error while allocating obfusecator %v: %v", name, err)
		}
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
		vpn.tun_trans.SetIPv4(tun.ADDRESS, tun_server_addr)
		vpn.tun_trans.SetIPv4(tun.DST_ADDRESS, tun_client_addr)
	} else {
		vpn.tun_trans.SetIPv4(tun.ADDRESS, tun_client_addr)
		vpn.tun_trans.SetIPv4(tun.DST_ADDRESS, tun_server_addr)
	}

	vpn.tun_mtu = vpn.wire_mtu
	for _, obfusecator := range vpn.obfusecators {
		vpn.tun_mtu -= obfusecator.GetLengthDelta()
	}
	log.Printf("MTU for TUN transport is %d\n", vpn.tun_mtu)
	vpn.tun_trans.SetMTU(vpn.tun_mtu)

	return nil
}

func (vpn *VPN) Init(is_server bool, options map[string]interface{}) error {
	obfs_list := options["obfs"].([]interface{})
	if err := vpn.initObfusecators(obfs_list); err != nil {
		return err
	}
	wire_opt := options["wire"].(map[string]interface{})
	if err := vpn.initWireTransport(is_server, wire_opt); err != nil {
		return err
	}
	tun_opt := options["tunnel"].(map[string]interface{})
	if err := vpn.initTunTransport(is_server, tun_opt); err != nil {
		return err
	}

	log.Printf("VPN Init done: %v <-> %d obfs <-> %v\n",
		vpn.wire_trans, len(vpn.obfusecators), vpn.tun_trans)

	return nil
}

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

func (vpn *VPN) obfsEncode(plain_c <-chan []byte, obfsed_c chan<- []byte) {
	for {
		plain, ok := <-plain_c
		if !ok {
			log.Fatalf("Error reading from channel %v", plain_c)
			break
		}
		obfsed := plain
		for _, obfusecator := range vpn.obfusecators {
			obfsed = obfusecator.Encode(plain)
			plain = obfsed
		}
		obfsed_c <- obfsed
	}
}

func (vpn *VPN) obfsDecode(obfsed_c <-chan []byte, plain_c chan<- []byte) {
OuterLoop:
	for {
		obfsed, ok := <-obfsed_c
		if !ok {
			log.Fatalf("Error reading from channel %v", obfsed_c)
			break
		}
		plain := obfsed
		// reverse
		for i := len(vpn.obfusecators) - 1; i >= 0; i-- {
			var err error
			plain, err = vpn.obfusecators[i].Decode(obfsed)
			if err != nil {
				log.Printf("Error while decoding by %v, drop\n", vpn.obfusecators[i])
				continue OuterLoop
			}
			obfsed = plain
		}
		plain_c <- plain
	}
}

func (vpn *VPN) Start() {
	vpn.from_tun = make(chan []byte, VPN_CHANNEL_BUFFER)
	vpn.to_tun = make(chan []byte, VPN_CHANNEL_BUFFER)
	vpn.from_wire = make(chan []byte, VPN_CHANNEL_BUFFER)
	vpn.to_wire = make(chan []byte, VPN_CHANNEL_BUFFER)

	go readToChannel(vpn.tun_trans, vpn.tun_mtu, vpn.from_tun)
	go readToChannel(vpn.wire_trans, vpn.wire_mtu, vpn.from_wire)
	go writeFromChannel(vpn.tun_trans, vpn.to_tun)
	go writeFromChannel(vpn.wire_trans, vpn.to_wire)

	go vpn.obfsEncode(vpn.from_tun, vpn.to_wire)
	go vpn.obfsDecode(vpn.from_wire, vpn.to_tun)
}
