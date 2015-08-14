/*
* @Author: BlahGeek
* @Date:   2015-06-24
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-08-14
 */

package justvpn

import "io"
import "fmt"
import "net"
import "github.com/blahgeek/justvpn/tun"
import "github.com/blahgeek/justvpn/wire"
import "github.com/blahgeek/justvpn/obfs"
import "encoding/json"
import log "github.com/Sirupsen/logrus"

const VPN_CHANNEL_BUFFER = 64
const VPN_ERROR_COUNT_THRESHOLD = 32

type VPNOptions struct {
	Tunnel struct {
		Server string `json:"server"`
		Client string `json:"client"`
	} `json:"tunnel"`
	Wires []struct {
		Name    string          `json:"name"`
		Options json.RawMessage `json:"options"`
	} `json:"wires"`
	Obfs []struct {
		Name    string          `json:"name"`
		Options json.RawMessage `json:"options"`
	} `json:"obfs"`
	Route struct {
		Wire []string `json:"wire"`
		VPN  []string `json:"vpn"`
	} `json:"route"`
}

type VPN struct {
	from_tun, to_tun, from_wire, to_wire chan []byte

	wire_transports []wire.Transport
	wire_min_mtu    int

	tun_trans tun.Tun
	tun_mtu   int

	obfusecators []obfs.Obfusecator

	max_packet_cap int

	is_server bool
	options   VPNOptions
}

func (vpn *VPN) initObfusecators() error {
	vpn.obfusecators = make([]obfs.Obfusecator, len(vpn.options.Obfs))
	vpn.tun_mtu = vpn.wire_min_mtu
	for index, item := range vpn.options.Obfs {
		var err error
		vpn.obfusecators[index], err = obfs.New(item.Name, item.Options, vpn.tun_mtu)
		if err != nil {
			return fmt.Errorf("Error while allocating obfusecator %v: %v", item.Name, err)
		}
		obfs_max_plain_len := vpn.obfusecators[index].GetMaxPlainLength()
		log.WithFields(log.Fields{
			"name": item.Name,
			"old":  vpn.tun_mtu,
			"new":  obfs_max_plain_len,
		}).Debug("Updating MTU for obfusecator")
		vpn.tun_mtu = obfs_max_plain_len
	}
	return nil
}

func (vpn *VPN) initWireTransport() error {
	vpn.wire_transports = make([]wire.Transport, len(vpn.options.Wires))
	vpn.wire_min_mtu = -1
	for index, item := range vpn.options.Wires {
		var err error
		vpn.wire_transports[index], err = wire.New(item.Name, vpn.is_server, item.Options)
		if err != nil {
			return err
		}
		mtu := vpn.wire_transports[index].MTU()
		if vpn.wire_min_mtu == -1 || mtu < vpn.wire_min_mtu {
			vpn.wire_min_mtu = mtu
		}
	}
	log.WithField("mtu", vpn.wire_min_mtu).Info("MTU for wire transport detected")
	return nil
}

func (vpn *VPN) initTunTransport() error {
	var err error
	vpn.tun_trans, err = tun.New()
	if err != nil {
		return err
	}

	tun_server_addr := net.ParseIP(vpn.options.Tunnel.Server)
	tun_client_addr := net.ParseIP(vpn.options.Tunnel.Client)
	if vpn.is_server {
		log.WithFields(log.Fields{
			"local":  tun_server_addr,
			"remote": tun_client_addr,
		}).Info("Setting up TUN IP")
		vpn.tun_trans.SetIPv4(tun.ADDRESS, tun_server_addr)
		vpn.tun_trans.SetIPv4(tun.DST_ADDRESS, tun_client_addr)
	} else {
		log.WithFields(log.Fields{
			"local":  tun_client_addr,
			"remote": tun_server_addr,
		}).Info("Setting up TUN IP")
		vpn.tun_trans.SetIPv4(tun.ADDRESS, tun_client_addr)
		vpn.tun_trans.SetIPv4(tun.DST_ADDRESS, tun_server_addr)
	}

	log.WithField("mtu", vpn.tun_mtu).Info("Setting MTU for TUN transport")
	vpn.tun_trans.SetMTU(vpn.tun_mtu)

	return tun.ApplyInterfaceRouter(vpn.tun_trans)
}

func (vpn *VPN) initRouter() error {
	var err error
	var wire_gw, vpn_gw net.IP

	wire_gw, err = tun.GetWireDefaultGateway()
	if err != nil {
		return err
	}
	vpn_gw, err = vpn.tun_trans.GetIPv4(tun.DST_ADDRESS)
	if err != nil {
		return err
	}
	log.WithFields(log.Fields{
		"wire_gw": wire_gw,
		"vpn_gw":  vpn_gw,
	}).Info("Default gateway for non-VPN and VPN traffic")

	var wire_rules, vpn_rules []net.IPNet

	for _, wire_trans := range vpn.wire_transports {
		nets := wire_trans.GetWireNetworks()
		log.WithFields(log.Fields{
			"wire":     wire_trans,
			"networks": nets,
		}).Debug("Setting router for wire transport")
		wire_rules = append(wire_rules, nets...)
	}

	for _, rule := range vpn.options.Route.Wire {
		if _, rule_net, rule_err := net.ParseCIDR(rule); rule_err == nil {
			wire_rules = append(wire_rules, *rule_net)
		}
	}
	for _, rule := range vpn.options.Route.VPN {
		if _, rule_net, rule_err := net.ParseCIDR(rule); rule_err == nil {
			vpn_rules = append(vpn_rules, *rule_net)
		}
	}

	return tun.ApplyRouter(wire_rules, vpn_rules, wire_gw, vpn_gw, false)
}

func (vpn *VPN) Init(is_server bool, options []byte) error {
	vpn.is_server = is_server
	if err := json.Unmarshal(options, &vpn.options); err != nil {
		return err
	}

	if err := vpn.initWireTransport(); err != nil {
		return err
	}
	if err := vpn.initObfusecators(); err != nil {
		return err
	}
	if err := vpn.initTunTransport(); err != nil {
		return err
	}
	if !is_server {
		if err := vpn.initRouter(); err != nil {
			return err
		}
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
	log.WithField("capacity", vpn.max_packet_cap).Debug("Using MAX packet capacity")
	log.WithFields(log.Fields{
		"wires": len(vpn.wire_transports),
		"obfs":  len(vpn.obfusecators),
	}).Info("VPN Init done")

	return nil
}

func (vpn *VPN) readToChannel(reader io.Reader, mtu int, c chan<- []byte) {

	defer func() {
		log.WithField("reader", reader).Error("Read failed")
	}()

	var error_count int = 0
	for {
		buf := make([]byte, mtu, vpn.max_packet_cap)
		if rdlen, err := reader.Read(buf); rdlen == 0 || err != nil {
			error_count += 1
			log.WithFields(log.Fields{
				"reader": reader,
				"error":  err,
				"count":  error_count,
			}).Warning("Error reading from reader")
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
		log.WithField("writer", writer).Error("Write failed")
	}()

	var error_count int = 0
	for {
		buf, ok := <-c
		if !ok {
			break
		}
		if wlen, err := writer.Write(buf); wlen != len(buf) || err != nil {
			error_count += 1
			log.WithFields(log.Fields{
				"writer": writer,
				"error":  err,
				"count":  error_count,
			}).Warning("Error writing to writer")
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
		log.Error("Exit obfs encoding")
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
		log.Error("Exit obfs decoding")
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
				log.WithField("obfusecator", vpn.obfusecators[i]).
					Warning("Error decoding, drop it")
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
