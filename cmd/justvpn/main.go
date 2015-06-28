/*
* @Author: BlahGeek
* @Date:   2015-06-23
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-06-28
 */

package main

import "os"
import "os/signal"
import "io/ioutil"
import "encoding/json"
import "fmt"
import "log"
import "flag"
import "runtime/pprof"
import "github.com/blahgeek/justvpn"

func main() {

	need_help := flag.Bool("h", false, "Show help")
	is_server := flag.Bool("s", false, "Run as server")
	cpuprofile := flag.String("cpuprofile", "", "Write cpu profile to file")
	flag.Parse()
	if *need_help {
		fmt.Printf("Usage: %v [OPTIONS] config.json\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(0)
	}
	if *is_server {
		log.Println("Running as server!")
	}
	if *cpuprofile != "" {
		fmt.Printf("Saving CPU profile to %v", *cpuprofile)
		if f, err := os.Create(*cpuprofile); err != nil {
			log.Fatal(err)
		} else {
			pprof.StartCPUProfile(f)
			defer pprof.StopCPUProfile()
		}
	}

	if flag.NArg() == 0 {
		log.Fatal("Config file missing")
	}

	var options map[string]interface{}
	if json_content, err := ioutil.ReadFile(flag.Arg(0)); err != nil {
		log.Fatalf("Config file `%v` not found")
	} else {
		err = json.Unmarshal(json_content, &options)
		if err != nil {
			log.Fatalf("Error parsing config file: %v", err)
		}
	}

	vpn := justvpn.VPN{}
	if err := vpn.Init(*is_server, options); err != nil {
		log.Fatalf("Error initing VPN: %v", err)
	}

	vpn.Start()

	signal_chan := make(chan os.Signal, 1)
	signal.Notify(signal_chan, os.Interrupt)

	select {
	case <-signal_chan:
		fmt.Println("CTRL-C Pressed")
	}
}
