/*
* @Author: BlahGeek
* @Date:   2015-06-23
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-06-23
 */

package main

import "os"
import "fmt"
import "net"
import "github.com/blahgeek/justvpn/tun"

func main() {
	is_server := false
	if os.Args[1] == "server" {
		is_server = true
	}

	var x tun.Tun
	var err error
	if is_server {
		x, err = tun.New("tun2")
	} else {
		x, err = tun.New("tun3")
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

	go func() {
		for {
			buf, err := x.Read(1500)
			if err != nil {
				fmt.Printf("Error when reading: %v", err)
				break
			}
			fmt.Printf("Read %v bytes\n", len(buf))
		}
	}()

	select {}
}
