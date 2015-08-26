/*
* @Author: BlahGeek
* @Date:   2015-08-25
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-08-26
 */

package wire

import "encoding/binary"
import "bytes"
import "fmt"
import "math"
import "strings"
import log "github.com/Sirupsen/logrus"

type DNSHeader struct {
	Id                                 uint16
	Flag0, Flag1                       uint8
	Qdcount, Ancount, Nscount, Arcount uint16
}

const (
	DNS_MAX_LABEL_LENGTH       = 63
	DNS_MAX_NAME_LENGTH        = 255
	DNS_MAX_TEXT_SINGLE_LENGTH = 255
	DNS_MAX_TEXT_LENGTH        = 255

	_DNS_MAX_PACKET_SIZE = 512
)

var _IN_TXT = []byte("\x00\x10\x00\x01")
var _POINTER_TO_LABELS = []byte("\xc0\x0c")

type DNSPacketFactory struct {
	base_labels         [][]byte
	base_labels_bytes   []byte
	max_domain_data_len int

	logger *log.Entry
}

func NewDNSPacketFactory(base_domain string) (*DNSPacketFactory, error) {
	ret := new(DNSPacketFactory)
	ret.logger = log.WithField("logger", "DNSPacketFactory")

	base_labels_len := 0
	for _, label_str := range strings.Split(base_domain, ".") {
		if len(label_str) == 0 {
			continue
		}
		if len(label_str) > DNS_MAX_LABEL_LENGTH {
			return nil, fmt.Errorf("Domain label too long: %v", label_str)
		}
		ret.base_labels = append(ret.base_labels, []byte(label_str))
		base_labels_len += len(label_str) + 1
	}
	ret.max_domain_data_len = DNS_MAX_NAME_LENGTH - base_labels_len -
		int(math.Ceil(float64(base_labels_len)/DNS_MAX_LABEL_LENGTH))
	if ret.max_domain_data_len < 16 {
		return nil, fmt.Errorf("Base domain too long: %v", base_domain)
	}

	buf := new(bytes.Buffer)
	for _, label := range ret.base_labels {
		buf.WriteByte(byte(len(label)))
		buf.Write(label)
	}
	buf.WriteByte(0x00)
	ret.base_labels_bytes = buf.Bytes()

	ret.logger.WithFields(log.Fields{
		"domain":              base_domain,
		"max_domain_data_len": ret.max_domain_data_len,
	}).Debug("DNSPacketFactory inited")

	return ret, nil
}

func (p *DNSPacketFactory) writeDomain(buf *bytes.Buffer, data []byte) {
	if len(data) > p.max_domain_data_len {
		data = data[:p.max_domain_data_len]
	}
	for i := 0; i < len(data); i += DNS_MAX_LABEL_LENGTH {
		_end := i + DNS_MAX_LABEL_LENGTH
		if _end > len(data) {
			_end = len(data)
		}
		buf.WriteByte(byte(_end - i))
		buf.Write(data[i:_end])
	}
	buf.Write(p.base_labels_bytes)
}

func (p *DNSPacketFactory) readDomain(buf *bytes.Buffer) ([]byte, error) {
	domain_bytes, err := buf.ReadBytes(0x00)
	base_domain_index := len(domain_bytes) - len(p.base_labels_bytes)
	if err != nil || base_domain_index <= 0 {
		return nil, fmt.Errorf("Malformed dns packet")
	}
	if bytes.Compare(domain_bytes[base_domain_index:], p.base_labels_bytes) != 0 {
		return nil, fmt.Errorf("Base domain does not match: %v", string(domain_bytes))
	}

	ret := make([]byte, 0, base_domain_index)
	for i := 0; i+1 < base_domain_index; {
		_len := int(domain_bytes[i])
		ret = append(ret, domain_bytes[i+1:i+1+_len]...)
		i += (1 + _len)
	}
	return ret, nil
}

func (p *DNSPacketFactory) MakeDNSQuery(id uint16, data []byte) []byte {
	buf := new(bytes.Buffer)
	buf.Grow(_DNS_MAX_PACKET_SIZE)

	header := DNSHeader{Id: id, Qdcount: 1, Flag0: 0x01} // Recursive
	binary.Write(buf, binary.BigEndian, header)

	p.writeDomain(buf, data)
	buf.Write(_IN_TXT)
	return buf.Bytes()
}

func (p *DNSPacketFactory) ParseDNSQuery(msg []byte) (uint16, []byte, error) {
	var header DNSHeader
	buf := bytes.NewBuffer(msg)
	if err := binary.Read(buf, binary.BigEndian, &header); err != nil {
		return 0, nil, err
	}
	if header.Qdcount != 1 {
		return 0, nil, fmt.Errorf("DNSQuery.Qdcount != 1")
	}
	data, err := p.readDomain(buf)
	if err != nil || len(data) == 0 {
		return 0, nil, fmt.Errorf("Error parsing DNS query: %v", err)
	}
	return header.Id, data, nil
}

func (p *DNSPacketFactory) MakeDNSResult(id uint16, domain_data []byte, ttl uint32, data []byte) []byte {
	buf := new(bytes.Buffer)
	buf.Grow(_DNS_MAX_PACKET_SIZE)

	header := DNSHeader{Id: id, Qdcount: 1, Ancount: 1, Flag0: 0x80} // Response
	binary.Write(buf, binary.BigEndian, header)

	p.writeDomain(buf, domain_data)
	buf.Write(_IN_TXT)

	buf.Write(_POINTER_TO_LABELS)
	buf.Write(_IN_TXT)
	binary.Write(buf, binary.BigEndian, ttl)

	data_len := len(data) + int(math.Ceil(float64(len(data))/DNS_MAX_TEXT_SINGLE_LENGTH))
	binary.Write(buf, binary.BigEndian, uint16(data_len))
	for i := 0; i < len(data); i += DNS_MAX_TEXT_SINGLE_LENGTH {
		_end := i + DNS_MAX_TEXT_SINGLE_LENGTH
		if _end > len(data) {
			_end = len(data)
		}
		buf.WriteByte(byte(_end - i))
		buf.Write(data[i:_end])
	}
	return buf.Bytes()
}

func (p *DNSPacketFactory) ParseDNSResult(msg []byte) (uint16, []byte, error) {
	buf := bytes.NewBuffer(msg)

	var header DNSHeader
	if err := binary.Read(buf, binary.BigEndian, &header); err != nil {
		return 0, nil, err
	}
	if header.Ancount != 1 {
		return 0, nil, fmt.Errorf("DNSQuery.Ancount != 1")
	}
	if _, err := p.readDomain(buf); err != nil {
		return 0, nil, err
	}

	var unused [14]byte
	if _, err := buf.Read(unused[:]); err != nil || bytes.Compare(unused[4:6], _POINTER_TO_LABELS) != 0 {
		return 0, nil, fmt.Errorf("Malformed pakcet")
	}

	var data_len uint16
	if err := binary.Read(buf, binary.BigEndian, &data_len); err != nil || data_len <= 1 {
		return 0, nil, fmt.Errorf("Malformed packet")
	}
	ret := make([]byte, 0, data_len)
	for i := 0; i < int(data_len); {
		this_len, _ := buf.ReadByte()
		this_txt := make([]byte, int(this_len))
		buf.Read(this_txt)
		ret = append(ret, this_txt...)
		i += int(this_len) + 1
	}
	return header.Id, ret, nil
}
