/*
* @Author: BlahGeek
* @Date:   2015-08-25
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-08-26
 */

package wire

import "testing"
import "bytes"

func TestDnsQuery(t *testing.T) {
	fac, err := NewDNSPacketFactory("blahgeek.com")
	if err != nil {
		t.Fatal(err)
	}
	query := fac.MakeDNSQuery(0xDEAD, []byte("www"))
	query_bytes := []byte("\xde\xad\x01\x00\x00\x01\x00\x00\x00\x00\x00\x00" +
		"\x03www\x08blahgeek\x03com\x00\x00\x10\x00\x01")
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
