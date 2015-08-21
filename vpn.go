/*
* @Author: BlahGeek
* @Date:   2015-06-24
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-08-21
 */

package justvpn

import "io"
import "fmt"
import "net"
import "sync"
import "github.com/blahgeek/justvpn/tun"
import "github.com/blahgeek/justvpn/wire"
import "github.com/blahgeek/justvpn/obfs"
import "encoding/json"
import log "github.com/Sirupsen/logrus"

const VPN_CHANNEL_BUFFER = 64

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
	waiter                               sync.WaitGroup

	// saved route rules
	wire_rules, vpn_rules []net.IPNet
	wire_gw, vpn_gw       net.IP

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
	vpn.tun_mtu = vpn.wire_min_mtu
	for _, item := range vpn.options.Obfs {
		obfusecator, err := obfs.New(item.Name, item.Options, vpn.tun_mtu)
		if err != nil {
			return fmt.Errorf("Error while allocating obfusecator %v: %v", item.Name, err)
		}
		obfs_max_plain_len := obfusecator.GetMaxPlainLength()
		log.WithFields(log.Fields{
			"name": item.Name,
			"old":  vpn.tun_mtu,
			"new":  obfs_max_plain_len,
		}).Debug("Updating MTU for obfusecator")
		vpn.tun_mtu = obfs_max_plain_len
		vpn.obfusecators = append(vpn.obfusecators, obfusecator)
	}
	return nil
}

func (vpn *VPN) initWireTransport() error {
	vpn.wire_min_mtu = -1
	for _, item := range vpn.options.Wires {
		wire_trans, err := wire.New(item.Name, vpn.is_server, item.Options)
		if err != nil {
			return err
		}
		mtu := wire_trans.MTU()
		if vpn.wire_min_mtu == -1 || mtu < vpn.wire_min_mtu {
			vpn.wire_min_mtu = mtu
		}
		vpn.wire_transports = append(vpn.wire_transports, wire_trans)
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
	if tun_server_addr == nil || tun_client_addr == nil {
		return fmt.Errorf("Invalid TUN Address")
	}
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

	vpn.wire_gw, err = tun.GetWireDefaultGateway()
	if err != nil {
		return err
	}
	vpn.vpn_gw, err = vpn.tun_trans.GetIPv4(tun.DST_ADDRESS)
	if err != nil {
		return err
	}
	log.WithFields(log.Fields{
		"wire_gw": vpn.wire_gw,
		"vpn_gw":  vpn.vpn_gw,
	}).Info("Default gateway for non-VPN and VPN traffic")

	for _, wire_trans := range vpn.wire_transports {
		nets := wire_trans.GetWireNetworks()
		log.WithFields(log.Fields{
			"wire":     wire_trans,
			"networks": nets,
		}).Debug("Setting router for wire transport")
		vpn.wire_rules = append(vpn.wire_rules, nets...)
	}

	for _, rule := range vpn.options.Route.Wire {
		if _, rule_net, rule_err := net.ParseCIDR(rule); rule_err == nil {
			vpn.wire_rules = append(vpn.wire_rules, *rule_net)
		}
	}
	for _, rule := range vpn.options.Route.VPN {
		if _, rule_net, rule_err := net.ParseCIDR(rule); rule_err == nil {
			vpn.vpn_rules = append(vpn.vpn_rules, *rule_net)
		}
	}

	return tun.ApplyRouter(vpn.wire_rules, vpn.vpn_rules,
		vpn.wire_gw, vpn.vpn_gw, false)
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
		log.WithField("reader", reader).Warning("Reading from reader exited")
		vpn.waiter.Done()
	}()

	for {
		buf := make([]byte, mtu, vpn.max_packet_cap)
		if rdlen, err := reader.Read(buf); err != nil {
			log.WithFields(log.Fields{
				"reader": reader,
				"error":  err,
			}).Warning("Error reading from reader, exit")
			break
		} else if rdlen == 0 {
			log.WithField("reader", reader).Warning("Read zero byte from reader, ignore")
		} else {
			c <- buf[:rdlen]
		}
	}
}

func (vpn *VPN) writeFromChannel(writer io.Writer, c <-chan []byte) {

	defer func() {
		log.WithField("writer", writer).Warning("Writing to writer exited")
		vpn.waiter.Done()
	}()

	for {
		buf, ok := <-c
		if !ok {
			break
		}
		if wlen, err := writer.Write(buf); err != nil {
			log.WithFields(log.Fields{
				"writer": writer,
				"error":  err,
			}).Warning("Error writing to writer, exit")
			break
		} else if wlen != len(buf) {
			log.WithFields(log.Fields{
				"writer":    writer,
				"buf_len":   len(buf),
				"write_len": wlen,
			}).Warning("Not all bytes is wrotten into writer, ignore")
		}
	}
}

func (vpn *VPN) obfsEncode(plain_c <-chan []byte, obfsed_c chan<- []byte) {

	defer func() {
		log.Warning("Obfusecator encoding worker exited")
		vpn.waiter.Done()
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
		log.Warning("Obfusecator decoding worker exited")
		vpn.waiter.Done()
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
	vpn.waiter = sync.WaitGroup{}

	vpn.from_wire = make(chan []byte, VPN_CHANNEL_BUFFER)
	vpn.to_wire = make(chan []byte, VPN_CHANNEL_BUFFER)
	for _, wire_trans := range vpn.wire_transports {
		vpn.waiter.Add(2)
		go vpn.readToChannel(wire_trans, vpn.wire_min_mtu, vpn.from_wire)
		go vpn.writeFromChannel(wire_trans, vpn.to_wire)
	}

	vpn.from_tun = make(chan []byte, VPN_CHANNEL_BUFFER)
	vpn.to_tun = make(chan []byte, VPN_CHANNEL_BUFFER)
	vpn.waiter.Add(2)
	go vpn.readToChannel(vpn.tun_trans, vpn.tun_mtu, vpn.from_tun)
	go vpn.writeFromChannel(vpn.tun_trans, vpn.to_tun)

	vpn.waiter.Add(2)
	go vpn.obfsEncode(vpn.from_tun, vpn.to_wire)
	go vpn.obfsDecode(vpn.from_wire, vpn.to_tun)
}

func (vpn *VPN) Destroy() {
	log.Warning("Stopping VPN service")
	close_not_nil := func(x chan []byte) {
		if x != nil {
			close(x)
		}
	}
	close_not_nil(vpn.from_wire)
	close_not_nil(vpn.from_tun)
	close_not_nil(vpn.to_wire)
	close_not_nil(vpn.to_tun)

	var err error

	err = tun.ApplyRouter(vpn.wire_rules, vpn.vpn_rules,
		vpn.wire_gw, vpn.vpn_gw, true)
	log.WithField("error", err).Info("Route rules deleted")

	if vpn.tun_trans != nil {
		err = vpn.tun_trans.Destroy()
		log.WithField("error", err).Info("TUN device destroyed")
	}
	for _, obfs := range vpn.obfusecators {
		err = obfs.Close()
		log.WithFields(log.Fields{
			"obfs":  obfs,
			"error": err,
		}).Info("Obfusecator closed")
	}
	for _, wire_trans := range vpn.wire_transports {
		err = wire_trans.Close()
		log.WithFields(log.Fields{
			"wire":  wire_trans,
			"error": err,
		}).Info("Wire transport closed")
	}
}
