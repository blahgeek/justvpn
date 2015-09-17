/*
* @Author: BlahGeek
* @Date:   2015-08-25
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-09-17
 */

package wire

import "bytes"
import "encoding/base32"
import "testing"
import "math/rand"
import "github.com/miekg/dns"

func TestDNSUpstreamEncoding(t *testing.T) {
	var tmp [60]byte
	bad_domain := "www." + base32.StdEncoding.EncodeToString(tmp[:]) + "."

	if _, good := dns.IsDomainName(bad_domain); good {
		t.Errorf("Bad domain not detected: %v", bad_domain)
	}

	codec, err := NewDNSTransportUpstreamCodec("blahgeek.com")
	if err != nil {
		t.Fatalf("Unable to build codec: %v", err)
	}
	streamer := DNSTransportStream{codec: codec}

	random := rand.New(rand.NewSource(0))
	for i := 0; i < 1500; i += 1 {
		var msg []byte
		for j := 0; j < i; j += 1 {
			msg = append(msg, byte(random.Int()&0xff))
		}
		var decoded_msg []byte
		upstreams := streamer.Encode(msg)
		for _, d := range upstreams {
			t.Logf("Encoded domain for msg length %v: %v", i, d)
			if _, good := dns.IsDomainName(d); !good {
				t.Errorf("Bad domain for msg length %v", i)
			}
			decoded_msg = streamer.Decode(d)
		}
		if bytes.Compare(decoded_msg, msg) != 0 {
			t.Errorf("Decoded msg != msg")
		}
	}
}

func TestDNSDownstreamEncoding(t *testing.T) {
	streamer := DNSTransportStream{codec: &DNSTransportDownstreamCodec{}}

	random := rand.New(rand.NewSource(0))
	for i := 0; i < 1500; i += 1 {
		var msg []byte
		for j := 0; j < i; j += 1 {
			msg = append(msg, byte(random.Int()&0xff))
		}
		var decoded_msg []byte
		upstreams := streamer.Encode(msg)
		for _, d := range upstreams {
			t.Logf("Encoded TXT for msg length %v: %v", i, d)
			if len(d) > DNS_MAX_TXT_LENGTH {
				t.Errorf("TXT too long for msg length %v", i)
			}
			decoded_msg = streamer.Decode(d)
		}
		if bytes.Compare(decoded_msg, msg) != 0 {
			t.Errorf("Decoded msg != msg")
		}
	}
}

func TestDNSCmpSeq(t *testing.T) {
	if !_cmp_seq(1, 2) {
		t.Errorf("1 < 2")
	}
	if _cmp_seq(42, 2) {
		t.Errorf("42 > 2")
	}
	if !_cmp_seq(0x07ffffff, 2) {
		t.Errorf("0x07ffffff < 2")
	}
	if _cmp_seq(0x07ffffff, 0x07f00090) {
		t.Errorf("0x07ffffff > 0x07f00090")
	}
}
