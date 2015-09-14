/*
* @Author: BlahGeek
* @Date:   2015-08-25
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-09-14
 */

package wire

import "fmt"
import "encoding/binary"
import "encoding/base32"
import "github.com/miekg/dns"

// 32-bit Seq: (31 downto 5): seq, (4 downto 1): fragment No., (0): more fragment
const DNS_UPSTREAM_SEQ_FRAGMENT_BIT = 4 // 16 fragments at most
const DNS_UPSTREAM_MAX_FRAGMENT = 16

const DNS_UPSTREAM_WINDOW_SIZE = 64

const DNS_MAX_BITS_PER_LABEL = 35 // base32, max 63 char per label

type DNSTransportUtility struct {
	Domain        string
	domain_labels int

	MaxBitsPerName int

	upstream_seq uint32

	upstream_recv_window [DNS_UPSTREAM_WINDOW_SIZE]struct {
		in_use         bool
		seq            uint32
		fragments      [DNS_UPSTREAM_MAX_FRAGMENT][]byte
		fragments_bits uint32
		fragment_count uint32
	}
}

func NewDNSTransportUtility(domain string) (*DNSTransportUtility, error) {
	ret := DNSTransportUtility{Domain: domain + "."}

	var ok bool
	if ret.domain_labels, ok = dns.IsDomainName(ret.Domain); !ok {
		return nil, fmt.Errorf("Bad base domain %s", domain)
	}

	name_len := 255 - len(ret.Domain)
	name_len -= 9 // seq number (4 byte -> 8 byte) + '.'

	ret.MaxBitsPerName = name_len / 64 * DNS_MAX_BITS_PER_LABEL
	if tmp := name_len % 64; tmp > 9 {
		ret.MaxBitsPerName += (tmp - 1) / 8 * 5
	}

	return &ret, nil
}

func (x *DNSTransportUtility) EncodeUpstreamSingle(msg []byte,
	segment uint32, more_segment bool) string {

	seq_uint32 := uint32(x.upstream_seq << (DNS_UPSTREAM_SEQ_FRAGMENT_BIT + 1))
	seq_uint32 |= (segment << 1)
	if more_segment {
		seq_uint32 |= 0x1
	}
	var seq_bytes [4]byte
	binary.BigEndian.PutUint32(seq_bytes[:], seq_uint32)

	ret := base32.StdEncoding.EncodeToString(seq_bytes[:])

	for i := 0; i < len(msg); i += DNS_MAX_BITS_PER_LABEL {
		j := i + DNS_MAX_BITS_PER_LABEL
		if j > len(msg) {
			j = len(msg)
		}
		ret += "." + base32.StdEncoding.EncodeToString(msg[i:j])
	}
	ret += "." + x.Domain
	return ret
}

func (x *DNSTransportUtility) EncodeUpstream(msg []byte) []string {
	var ret []string
	var segment uint32 = 0
	for i := 0; i < len(msg); i += x.MaxBitsPerName {
		j := i + x.MaxBitsPerName
		if j > len(msg) {
			j = len(msg)
		}
		ret = append(ret, x.EncodeUpstreamSingle(msg[i:j], segment, j != len(msg)))
		segment += 1
	}
	x.upstream_seq += 1
	return ret
}

func (x *DNSTransportUtility) DecodeUpstreamSingle(msg string) ([]byte, uint32, uint32, bool) {
	var ret []byte

	labels := dns.SplitDomainName(msg)
	if len(labels) <= 1+x.domain_labels {
		return nil, 0, 0, false
	}

	var seq uint32
	if seq_bytes, err := base32.StdEncoding.DecodeString(labels[0]); err == nil {
		seq = binary.BigEndian.Uint32(seq_bytes)
	} else {
		return nil, 0, 0, false
	}

	labels = labels[1 : len(labels)-x.domain_labels]

	for _, label := range labels {
		data, err := base32.StdEncoding.DecodeString(label)
		if err == nil {
			ret = append(ret, data...)
		}
	}

	var more_fragment bool = ((seq & 0x01) == 1)
	return ret,
		seq >> (DNS_UPSTREAM_SEQ_FRAGMENT_BIT + 1),
		(seq & ((1 << (DNS_UPSTREAM_SEQ_FRAGMENT_BIT + 1)) - 1)) >> 1,
		more_fragment
}

// return x < y
func _cmp_seq(x, y uint32) bool {
	max_div_2 := (((1 << (32 - 1 - DNS_UPSTREAM_SEQ_FRAGMENT_BIT)) - 1) >> 1)
	if (y > x && int(y-x) < max_div_2) || (y < x && int(x-y) > max_div_2) {
		return true
	} else {
		return false
	}
}

// return nil if no whole packet is available
func (x *DNSTransportUtility) DecodeUpstream(msg string) []byte {
	dat, seq, frag, more_frag := x.DecodeUpstreamSingle(msg)
	if dat == nil {
		return nil
	}

	box := &x.upstream_recv_window[seq%DNS_UPSTREAM_WINDOW_SIZE]
	if box.in_use && _cmp_seq(seq, box.seq) {
		// this packet is too late
		return nil
	}
	if box.in_use && seq == box.seq && (box.fragments_bits&(1<<frag)) != 0 {
		return nil
	}
	if !box.in_use || box.seq != seq {
		// clear this box
		box.in_use = true
		box.seq = seq
		box.fragments_bits = 0
		box.fragment_count = 0
	}
	box.fragments_bits |= (1 << frag)
	box.fragments[frag] = dat
	if !more_frag {
		box.fragment_count = frag + 1
	}

	var ret []byte
	if box.fragment_count != 0 && ((1<<box.fragment_count)-1) == box.fragments_bits {
		// all fragments is here
		for i := 0; i < int(box.fragment_count); i += 1 {
			ret = append(ret, box.fragments[i]...)
		}
		box.in_use = false
	}

	return ret
}
