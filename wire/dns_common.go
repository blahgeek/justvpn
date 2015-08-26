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

type DNSHeader struct {
	Id                                 uint16
	Flag0, Flag1                       uint8
	Qdcount, Ancount, Nscount, Arcount uint16
}

const (
	DNS_MAX_LABEL_LENGTH     = 63
	DNS_MAX_NAME_LENGTH      = 255
	DNS_MAX_TXT_LENGTH       = 255
	DNS_MAX_TXT_TOTAL_LENGTH = 255
)

var _IN_TXT = []byte("\x00\x10\x00\x01")

func writeDomainName(buf *bytes.Buffer, labels [][]byte) {
	name_length := 0
	for _, label := range labels {
		length := len(label)
		if length > DNS_MAX_LABEL_LENGTH {
			length = DNS_MAX_LABEL_LENGTH
		}
		if length > (DNS_MAX_NAME_LENGTH - name_length - 1) {
			length = DNS_MAX_NAME_LENGTH - name_length - 1
		}
		if length <= 0 {
			break // not enough space
		}
		buf.WriteByte(byte(length))
		buf.Write(label[:length])
		name_length += length
	}
	buf.WriteByte(0x00)
}

func readDomainName(buf *bytes.Buffer) ([][]byte, error) {
	var labels [][]byte
	for {
		length, err := buf.ReadByte()
		if err != nil {
			return labels, err
		}
		if length == 0 {
			break
		}
		label := make([]byte, int(length))
		var n int
		n, err = buf.Read(label)
		if n != int(length) || err != nil {
			return labels, nil
		}
		labels = append(labels, label)
	}
	return labels, nil
}

func MakeDNSQuery(id uint16, labels [][]byte) []byte {
	buf := new(bytes.Buffer)

	header := DNSHeader{Id: id, Qdcount: 1}
	header.Flag0 |= 1 // Recursive Desired
	binary.Write(buf, binary.BigEndian, header)

	writeDomainName(buf, labels)
	buf.Write(_IN_TXT)
	return buf.Bytes()
}

func ParseDNSQuery(msg []byte) (uint16, [][]byte, error) {
	var header DNSHeader
	buf := bytes.NewBuffer(msg)
	if err := binary.Read(buf, binary.BigEndian, &header); err != nil {
		return 0, nil, err
	}
	if header.Qdcount != 1 {
		return 0, nil, fmt.Errorf("DNSQuery.Qdcount != 1")
	}
	labels, err := readDomainName(buf)
	if err != nil || len(labels) == 0 {
		return 0, nil, fmt.Errorf("Error parsing DNS query")
	}
	return header.Id, labels, nil
}
