/*
* @Author: BlahGeek
* @Date:   2015-06-23
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-08-14
 */

package main

import "os"
import "os/signal"
import "io/ioutil"
import "encoding/json"
import "fmt"
import "flag"
import "time"
import "bytes"
import "runtime/pprof"
import "github.com/blahgeek/justvpn"
import log "github.com/Sirupsen/logrus"

type LogFormatter struct {
	text_formatter *log.TextFormatter
}

func (f *LogFormatter) Format(entry *log.Entry) ([]byte, error) {
	logger := "JUSTVPN"
	if val, ok := entry.Data["logger"]; ok {
		logger = val.(string)
		delete(entry.Data, "logger")
	}
	for len(logger) < 7 {
		logger += " "
	}
	logger = logger[:7]
	prefix := bytes.NewBufferString(fmt.Sprintf("[%s] ", logger))
	output, err := f.text_formatter.Format(entry)
	prefix.Write(output)
	return prefix.Bytes(), err
}

func main() {

	need_help := flag.Bool("h", false, "Show help")
	is_server := flag.Bool("s", false, "Run as server")
	verbose := flag.Bool("v", false, "More verbose output")
	cpuprofile := flag.String("cpuprofile", "", "Write cpu profile to file")
	flag.Parse()

	log.SetFormatter(&LogFormatter{&log.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: time.RFC822,
	}})
	log.SetLevel(log.InfoLevel)
	if *verbose {
		log.SetLevel(log.DebugLevel)
	}
	if *need_help {
		fmt.Printf("Usage: %v [OPTIONS] config.json\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(0)
	}
	if *is_server {
		log.Info("Running as server!")
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
		log.WithField("filename", flag.Arg(0)).Fatal("Error reading config file")
	} else {
		err = json.Unmarshal(json_content, &options)
		if err != nil {
			log.WithField("filename", flag.Arg(0)).Fatal("Error parsing config file")
		}
	}

	vpn := justvpn.VPN{}
	if err := vpn.Init(*is_server, options); err != nil {
		log.WithField("error", err).Fatal("Error initing VPN")
	}

	vpn.Start()

	signal_chan := make(chan os.Signal, 1)
	signal.Notify(signal_chan, os.Interrupt)

	select {
	case <-signal_chan:
		fmt.Println("CTRL-C Pressed")
	}
}
