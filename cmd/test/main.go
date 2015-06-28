/*
* @Author: BlahGeek
* @Date:   2015-06-23
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-06-28
 */

package main

import "os"
import "os/signal"
import "encoding/json"
import "fmt"
import "log"
import "net"
import "github.com/blahgeek/justvpn/tun"
import "github.com/blahgeek/justvpn/wire"
import "github.com/blahgeek/justvpn"

import "runtime/pprof"

func main() {

	f, err := os.Create(os.Args[1] + ".prof")
	if err != nil {
		log.Fatal(err)
	}
	pprof.StartCPUProfile(f)
	defer pprof.StopCPUProfile()

	is_server := false
	if os.Args[1] == "server" {
		is_server = true
	}

	var x tun.Tun
	if is_server {
		x, err = tun.New("tun8")
	} else {
		x, err = tun.New("tun9")
	}

	if err != nil {
		fmt.Printf("Error: %v", err)
		return
	}

	if is_server {
		x.SetIPv4(tun.ADDRESS, net.ParseIP("10.42.0.1"))
		x.SetIPv4(tun.DST_ADDRESS, net.ParseIP("10.42.0.2"))
	} else {
		x.SetIPv4(tun.ADDRESS, net.ParseIP("10.42.0.2"))
		x.SetIPv4(tun.DST_ADDRESS, net.ParseIP("10.42.0.1"))
	}

	options_str := []byte(`{"server_addr": "127.0.0.1:5438"}`)
	var options map[string]interface{}
	json.Unmarshal(options_str, &options)

	var w wire.Transport
	w, err = wire.New("udp", is_server, options)
	if err != nil {
		panic(err)
	}

	justvpn.StartRoute(x, w)

	signal_chan := make(chan os.Signal, 1)
	signal.Notify(signal_chan, os.Interrupt)

	select {
	case <-signal_chan:
		fmt.Println("CTRL-C Pressed")
	}
}
