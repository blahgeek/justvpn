/*
* @Author: BlahGeek
* @Date:   2015-08-25
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-10-31
 */

package wire

import "fmt"
import "encoding/base32"
import "encoding/ascii85"
import "github.com/miekg/dns"
import "net"
import "github.com/blahgeek/justvpn/wire/bitcodec"

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

type DNSCodecHeader struct {
	Seq            uint32 `bits:"27"`
	FragmentNumber uint32 `bits:"4"`
	MoreFragment   byte   `bits:"1"`
}

const DNS_FRAGMENT_BIT = 4
const DNS_MAX_FRAGMENTS = 16

type DNSTransportCodec interface {
	Encode(msg []byte, header DNSCodecHeader) string
	// Decode an encoded message, return raw message, seq No., frag, more_frag
	Decode(msg string) ([]byte, DNSCodecHeader)
	// Get max length of raw message per packet
	GetMaxLength() int
}

const DNS_UPSTREAM_MAX_LEN_PER_LABEL = 35

type DNSTransportUpstreamCodec struct {
	domain             string
	domain_label_count int
	max_len_per_name   int
	header_codec       *bitcodec.Bitcodec
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
	ret.header_codec = bitcodec.NewBitcodec(&DNSCodecHeader{})

	return &ret, nil
}

func (x *DNSTransportUpstreamCodec) GetMaxLength() int {
	return x.max_len_per_name
}

func (x *DNSTransportUpstreamCodec) Encode(msg []byte, header DNSCodecHeader) string {
	header_bytes := x.header_codec.EncodeToBytes(&header)
	ret := base32.StdEncoding.EncodeToString(header_bytes[:])
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

func (x *DNSTransportUpstreamCodec) Decode(msg string) ([]byte, DNSCodecHeader) {
	var ret []byte
	var header DNSCodecHeader
	labels := dns.SplitDomainName(msg)
	if len(labels) < 1+x.domain_label_count {
		return nil, header
	}

	header_bytes, err := base32.StdEncoding.DecodeString(labels[0])
	if err != nil {
		return nil, header
	}
	x.header_codec.DecodeFromBytes(header_bytes, &header)

	labels = labels[1 : len(labels)-x.domain_label_count]
	for _, label := range labels {
		var data []byte
		data, err = base32.StdEncoding.DecodeString(label)
		if err == nil {
			ret = append(ret, data...)
		}
	}
	return ret, header
}

const DNS_MAX_TXT_LENGTH = 255

type DNSTransportDownstreamCodec struct {
	header_codec *bitcodec.Bitcodec
}

func NewDNSTransportDownstreamCodec() *DNSTransportDownstreamCodec {
	codec := DNSTransportDownstreamCodec{}
	codec.header_codec = bitcodec.NewBitcodec(&DNSCodecHeader{})
	return &codec
}

func (x *DNSTransportDownstreamCodec) GetMaxLength() int {
	// ascii85
	return (DNS_MAX_TXT_LENGTH - 5) / 5 * 4 // 5 byte for seq
}

func (x *DNSTransportDownstreamCodec) Encode(msg []byte, header DNSCodecHeader) string {
	header_bytes := x.header_codec.EncodeToBytes(&header)

	dst := make([]byte, ascii85.MaxEncodedLen(4+len(msg)))

	ret_len := ascii85.Encode(dst[0:5], header_bytes[:])
	ret_len += ascii85.Encode(dst[5:], msg)

	return string(dst[:ret_len])
}

func (x *DNSTransportDownstreamCodec) Decode(msg string) ([]byte, DNSCodecHeader) {
	src := []byte(msg)
	var header DNSCodecHeader

	var header_bytes [4]byte
	ndst, _, err := ascii85.Decode(header_bytes[:], src[0:5], true)
	if ndst != 4 || err != nil {
		return nil, header
	}
	x.header_codec.DecodeFromBytes(header_bytes[:], &header)

	ret := make([]byte, len(msg)/5*4)
	ndst, _, err = ascii85.Decode(ret, src[5:], true)
	if err != nil {
		return nil, header
	}

	return ret[:ndst], header
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
		var has_more_fragment byte = 1
		if j == len(msg) {
			has_more_fragment = 0
		}
		ret = append(ret, x.codec.Encode(msg[i:j], DNSCodecHeader{
			Seq:            x.send_seq,
			FragmentNumber: segment,
			MoreFragment:   has_more_fragment,
		}))
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
	dat, header := x.codec.Decode(msg)
	if dat == nil {
		return nil
	}

	box := &x.recv_window[header.Seq%DNS_STREAM_WINDOW_SIZE]
	if box.in_use && _cmp_seq(header.Seq, box.seq) {
		// this packet is too late
		return nil
	}
	if box.in_use && header.Seq == box.seq && (box.fragments_bits&(1<<header.FragmentNumber)) != 0 {
		return nil
	}
	if !box.in_use || box.seq != header.Seq {
		// clear this box
		box.in_use = true
		box.seq = header.Seq
		box.fragments_bits = 0
		box.fragment_count = 0
	}
	box.fragments_bits |= (1 << header.FragmentNumber)
	box.fragments[header.FragmentNumber] = dat
	if header.MoreFragment == 0 {
		box.fragment_count = header.FragmentNumber + 1
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
