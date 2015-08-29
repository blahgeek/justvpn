/*
* @Author: BlahGeek
* @Date:   2015-08-25
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-08-29
 */

package wire

import "testing"
import "bytes"

func TestDnsQuery(t *testing.T) {
	fac, err := NewDNSPacketFactory("blahgeek.com")
	if err != nil {
		t.Fatal(err)
	}
	another_fac, _ := NewDNSPacketFactory("www.blahgeek.com")

	query := fac.MakeDNSQuery(0xDEAD, []byte("www"))
	query_bytes := []byte("\xde\xad\x01\x00\x00\x01\x00\x00\x00\x00\x00\x00" +
		"\x03www\x08blahgeek\x03com\x00\x00\x10\x00\x01")
	if bytes.Compare(query, query_bytes) != 0 {
		t.Errorf("Make DNS Query error: %v\n", query)
	}
	query = another_fac.MakeDNSQuery(0xDEAD, nil)
	if bytes.Compare(query, query_bytes) != 0 {
		t.Errorf("Make DNS Query error: %v\n", query)
	}

	id, data, e := fac.ParseDNSQuery(query_bytes)
	if e != nil {
		t.Errorf("Parse DNS Query error: %v\n", e)
	}
	if id != 0xDEAD || bytes.Compare(data, []byte("www")) != 0 {
		t.Error("Parse DNS Query error\n")
	}
}

func TestDnsResult(t *testing.T) {
	fac, err := NewDNSPacketFactory(".com")
	if err != nil {
		t.Fatal(err)
	}

	var msg string = "v=spf1 mx include:zoho.com ~all"
	result := fac.MakeDNSResult(0x524d, []byte("blahgeek"), 600, []byte(msg))
	result_cmp := []byte("\x52\x4d\x80\x00\x00\x01\x00\x01" +
		"\x00\x00\x00\x00\x08\x62\x6c\x61" +
		"\x68\x67\x65\x65\x6b\x03\x63\x6f" +
		"\x6d\x00\x00\x10\x00\x01\xc0\x0c" +
		"\x00\x10\x00\x01\x00\x00\x02\x58" +
		"\x00\x20\x1f\x76\x3d\x73\x70\x66" +
		"\x31\x20\x6d\x78\x20\x69\x6e\x63" +
		"\x6c\x75\x64\x65\x3a\x7a\x6f\x68" +
		"\x6f\x2e\x63\x6f\x6d\x20\x7e\x61" +
		"\x6c\x6c")
	if bytes.Compare(result, result_cmp) != 0 {
		t.Errorf("Make DNS Result error: %x\n", result)
	}

	id, data, e := fac.ParseDNSResult(result_cmp)
	if e != nil {
		t.Errorf("Parse DNS Result error: %v\n", e)
	}
	if id != 0x524d || bytes.Compare(data, []byte(msg)) != 0 {
		t.Errorf("Parse DNS Result error: %v\n", data)
	}

	another_fac, _ := NewDNSPacketFactory("xxx.yyy")
	id, data, e = another_fac.ParseDNSQuery(result_cmp)
	if e == nil {
		t.Error("Parse error not detected")
	}
}
