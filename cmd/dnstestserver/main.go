/*
* @Author: BlahGeek
* @Date:   2015-08-29
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-10-31
 */

package main

import "fmt"
import "flag"
import "encoding/json"
import "github.com/blahgeek/justvpn/wire"
import log "github.com/Sirupsen/logrus"

func main() {
	domain := flag.String("d", "x.blax.me", "Base domain")
	port := flag.Int("p", 53530, "Listen port")
	flag.Parse()

	log.SetLevel(log.DebugLevel)

	options := wire.DNSTransportServerOptions{
		BaseDomain: *domain,
		Port:       *port,
	}
	options_str, err := json.Marshal(options)
	if err != nil {
		fmt.Printf("Unable to build option: %v\n", err)
		return
	}

	server := wire.DNSTransportServer{}
	if err = server.Open(json.RawMessage(options_str)); err != nil {
		fmt.Printf("Unable to open server: %v\n", err)
		return
	}

	for {
		data := <-server.UpstreamBuf
		server.DownstreamBuf <- data
	}
}
