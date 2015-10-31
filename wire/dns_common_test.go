/*
* @Author: BlahGeek
* @Date:   2015-08-25
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-10-31
 */

package wire

import "bytes"
import "encoding/base32"
import "testing"
import "math/rand"
import "time"
import "sync"
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

	for i := 0; i < 1500; i += 1 {
		var msg []byte
		for j := 0; j < i; j += 1 {
			msg = append(msg, byte(rand.Int()&0xff))
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
	streamer := DNSTransportStream{codec: NewDNSTransportDownstreamCodec()}

	for i := 0; i < 1500; i += 1 {
		var msg []byte
		for j := 0; j < i; j += 1 {
			msg = append(msg, byte(rand.Int()&0xff))
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

func TestDNSStream(t *testing.T) {
	streamer := DNSTransportStream{codec: NewDNSTransportDownstreamCodec()}
	pipe_in := make(chan string, 64)
	pipe_out := make(chan string, 64)
	var pipes_waiter sync.WaitGroup

	pipe_transfer := func() {
		for {
			msg, ok := <-pipe_in
			if !ok {
				break
			}
			time.Sleep((time.Duration)(rand.Int()%1000) * time.Microsecond)
			pipe_out <- msg
			if rand.Int()%4 == 0 {
				pipe_out <- msg
				// duplicate
			}
		}
		pipes_waiter.Done()
	}

	for i := 0; i < 64; i += 1 {
		pipes_waiter.Add(1)
		go pipe_transfer()
	}
	go func() {
		pipes_waiter.Wait()
		close(pipe_out)
	}()

	var msgs [][]byte
	for i := 0; i < 1024; i += 1 {
		msg_len := rand.Int() % 1500
		msgs = append(msgs, make([]byte, msg_len))
		for j := 0; j < msg_len; j += 1 {
			msgs[i][j] = byte(rand.Int() & 0xff)
		}
	}

	go func() {
		for _, msg := range msgs {
			encoded := streamer.Encode(msg)
			for _, x := range encoded {
				pipe_in <- x
			}
		}
		close(pipe_in)
	}()

	decoded_count := 0
	for {
		msg, ok := <-pipe_out
		if !ok {
			break
		}
		ret := streamer.Decode(msg)
		if ret != nil {
			t.Logf("Decoded msg, count = %v", decoded_count)
			decoded_count += 1
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
