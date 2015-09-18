/*
* @Author: BlahGeek
* @Date:   2015-08-25
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-09-18
 */

package wire

import "fmt"
import "encoding/binary"
import "encoding/base32"
import "encoding/ascii85"
import "github.com/miekg/dns"
import "net"

const DNS_MAX_UDP_SIZE = 1500

type DNSUDPConn struct {
	*net.UDPConn
}

func (conn *DNSUDPConn) WriteDNSToUDP(m *dns.Msg, addr *net.UDPAddr) error {
	out, err := m.Pack()
	if err != nil {
		return err
	}
	_, err = conn.WriteToUDP(out, addr)
	return err
}

func (conn *DNSUDPConn) ReadDNSFromUDP() (*dns.Msg, *net.UDPAddr, error) {
	buf := make([]byte, DNS_MAX_UDP_SIZE)
	rdlen, addr, err := conn.ReadFromUDP(buf)
	if rdlen == 0 || err != nil {
		return nil, nil, fmt.Errorf("Error reading from UDP: %v", err)
	}
	m := new(dns.Msg)
	if err = m.Unpack(buf[:rdlen]); err != nil {
		return nil, nil, err
	}
	return m, addr, nil
}

// 32-bit Seq: (31 downto 5): seq, (4 downto 1): fragment No., (0): more fragment
const DNS_FRAGMENT_BIT = 4
const DNS_MAX_FRAGMENTS = 16

type DNSTransportCodec interface {
	Encode(msg []byte, seq uint32, segment uint32, more_segment bool) string
	// Decode an encoded message, return raw message, seq No., frag, more_frag
	Decode(msg string) ([]byte, uint32, uint32, bool)
	// Get max length of raw message per packet
	GetMaxLength() int
}

func encode_seq(seq uint32, segment uint32, more_segment bool) [4]byte {
	var ret [4]byte
	ret_uint32 := uint32(seq << (DNS_FRAGMENT_BIT + 1))
	ret_uint32 |= (segment << 1)
	if more_segment {
		ret_uint32 |= 0x01
	}
	binary.BigEndian.PutUint32(ret[:], ret_uint32)
	return ret
}

func decode_seq(seq_bytes []byte) (uint32, uint32, bool) {
	seq := binary.BigEndian.Uint32(seq_bytes)
	var more_fragment bool = ((seq & 0x01) == 1)
	return seq >> (DNS_FRAGMENT_BIT + 1),
		(seq & ((1 << (DNS_FRAGMENT_BIT + 1)) - 1)) >> 1,
		more_fragment
}

const DNS_UPSTREAM_MAX_LEN_PER_LABEL = 35

type DNSTransportUpstreamCodec struct {
	domain             string
	domain_label_count int
	max_len_per_name   int
}

func NewDNSTransportUpstreamCodec(domain string) (*DNSTransportUpstreamCodec, error) {
	ret := DNSTransportUpstreamCodec{domain: domain + "."}
	var ok bool
	if ret.domain_label_count, ok = dns.IsDomainName(ret.domain); !ok {
		return nil, fmt.Errorf("Bad domain %d", domain)
	}
	name_len := 255 - len(ret.domain)
	name_len -= 9 // seq number (4 byte --base32--> 8byte) + '.'

	ret.max_len_per_name = name_len / 64 * DNS_UPSTREAM_MAX_LEN_PER_LABEL
	if tmp := name_len % 64; tmp > 9 {
		ret.max_len_per_name += (tmp - 1) / 8 * 5
	}

	return &ret, nil
}

func (x *DNSTransportUpstreamCodec) GetMaxLength() int {
	return x.max_len_per_name
}

func (x *DNSTransportUpstreamCodec) Encode(msg []byte,
	seq uint32, segment uint32, more_segment bool) string {
	seq_bytes := encode_seq(seq, segment, more_segment)
	ret := base32.StdEncoding.EncodeToString(seq_bytes[:])
	for i := 0; i < len(msg); i += DNS_UPSTREAM_MAX_LEN_PER_LABEL {
		j := i + DNS_UPSTREAM_MAX_LEN_PER_LABEL
		if j > len(msg) {
			j = len(msg)
		}
		ret += "." + base32.StdEncoding.EncodeToString(msg[i:j])
	}
	ret += "." + x.domain
	return ret
}

func (x *DNSTransportUpstreamCodec) Decode(msg string) ([]byte, uint32, uint32, bool) {
	var ret []byte
	labels := dns.SplitDomainName(msg)
	if len(labels) < 1+x.domain_label_count {
		return nil, 0, 0, false
	}

	seq_bytes, err := base32.StdEncoding.DecodeString(labels[0])
	if err != nil {
		return nil, 0, 0, false
	}
	seq, fragment, more_fragment := decode_seq(seq_bytes)

	labels = labels[1 : len(labels)-x.domain_label_count]
	for _, label := range labels {
		var data []byte
		data, err = base32.StdEncoding.DecodeString(label)
		if err == nil {
			ret = append(ret, data...)
		}
	}
	return ret, seq, fragment, more_fragment
}

const DNS_MAX_TXT_LENGTH = 255

type DNSTransportDownstreamCodec struct{}

func (x *DNSTransportDownstreamCodec) GetMaxLength() int {
	// ascii85
	return (DNS_MAX_TXT_LENGTH - 5) / 5 * 4 // 5 byte for seq
}

func (x *DNSTransportDownstreamCodec) Encode(msg []byte,
	seq uint32, segment uint32, more_segment bool) string {

	seq_bytes := encode_seq(seq, segment, more_segment)
	dst := make([]byte, ascii85.MaxEncodedLen(4+len(msg)))

	ret_len := ascii85.Encode(dst[0:5], seq_bytes[:])
	ret_len += ascii85.Encode(dst[5:], msg)

	return string(dst[:ret_len])
}

func (x *DNSTransportDownstreamCodec) Decode(msg string) ([]byte, uint32, uint32, bool) {
	src := []byte(msg)
	var seq_bytes [4]byte

	ndst, _, err := ascii85.Decode(seq_bytes[:], src[0:5], true)
	if ndst != 4 || err != nil {
		return nil, 0, 0, false
	}
	seq, fragment, more_fragment := decode_seq(seq_bytes[:])

	ret := make([]byte, len(msg)/5*4)
	ndst, _, err = ascii85.Decode(ret, src[5:], true)
	if err != nil {
		return nil, 0, 0, false
	}

	return ret[:ndst], seq, fragment, more_fragment
}

const DNS_STREAM_WINDOW_SIZE = 64

type DNSTransportStream struct {
	codec DNSTransportCodec

	send_seq    uint32
	recv_window [DNS_STREAM_WINDOW_SIZE]struct {
		in_use         bool
		seq            uint32
		fragments      [DNS_MAX_FRAGMENTS][]byte
		fragments_bits uint32
		fragment_count uint32
	}
}

func (x *DNSTransportStream) Encode(msg []byte) []string {
	var ret []string
	var segment uint32 = 0
	max_byte_per_seg := x.codec.GetMaxLength()
	for i := 0; i < len(msg); i += max_byte_per_seg {
		j := i + max_byte_per_seg
		if j > len(msg) {
			j = len(msg)
		}
		ret = append(ret, x.codec.Encode(msg[i:j], x.send_seq, segment, j != len(msg)))
		segment += 1
	}
	x.send_seq += 1
	return ret
}

// return x < y
func _cmp_seq(x, y uint32) bool {
	max_div_2 := (((1 << (32 - 1 - DNS_FRAGMENT_BIT)) - 1) >> 1)
	if (y > x && int(y-x) < max_div_2) || (y < x && int(x-y) > max_div_2) {
		return true
	} else {
		return false
	}
}

// return nil if no whole packet is available
func (x *DNSTransportStream) Decode(msg string) []byte {
	dat, seq, frag, more_frag := x.codec.Decode(msg)
	if dat == nil {
		return nil
	}

	box := &x.recv_window[seq%DNS_STREAM_WINDOW_SIZE]
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
