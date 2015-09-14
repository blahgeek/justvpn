/*
* @Author: BlahGeek
* @Date:   2015-08-29
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-09-13
 */

package wire

import "testing"
import "fmt"
import "sync"
import "strings"

import "github.com/miekg/dns"

func makeDnsTxtRequest(domain string, server string) (string, error) {
	var err error
	var conn *dns.Conn
	if conn, err = dns.Dial("udp", server); err != nil {
		return "", err
	}

	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(domain), dns.TypeTXT)
	m.RecursionDesired = true

	if err = conn.WriteMsg(m); err != nil {
		return "", err
	}

	var rm *dns.Msg
	if rm, err = conn.ReadMsg(); err != nil {
		return "", err
	}

	if txt, ok := rm.Answer[0].(*dns.TXT); !ok {
		return "", fmt.Errorf("No TXT Answer")
	} else {
		return strings.Join(txt.Txt, ""), nil
	}

}

func TestDnsRequest(t *testing.T) {
	var waiter sync.WaitGroup

	addr := "166.111.8.28:53"
	do_req := func(domain, result string) {
		if ret, err := makeDnsTxtRequest(domain, addr); err != nil {
			t.Errorf("Requesting DNS for %v error: %v", domain, err)
		} else {
			t.Logf("TXT fot %v: %v", domain, ret)
			if ret != result {
				t.Errorf("TXT for %v error: %v", domain, ret)
			}
		}
		waiter.Done()
	}

	tests := [][2]string{
		{"blahgeek.com", "v=spf1 include:spf.messagingengine.com -all"},
		{"google.com", "v=spf1 include:_spf.google.com ~all"},
		{"_netblocks.google.com", "v=spf1 ip4:64.18.0.0/20 ip4:64.233.160.0/19 ip4:66.102.0.0/20 ip4:66.249.80.0/20 ip4:72.14.192.0/18 ip4:74.125.0.0/16 ip4:108.177.8.0/21 ip4:173.194.0.0/16 ip4:207.126.144.0/20 ip4:209.85.128.0/17 ip4:216.58.192.0/19 ip4:216.239.32.0/19 ~all"},
	}

	for _, test := range tests {
		go do_req(test[0], test[1])
		waiter.Add(1)
	}
	waiter.Wait()
}
