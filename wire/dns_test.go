/*
* @Author: BlahGeek
* @Date:   2015-08-25
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-08-25
 */

package wire

import "testing"
import "bytes"

func TestDnsQuery(t *testing.T) {
	query := MakeDNSQuery(0xDEAD, [][]byte{
		[]byte("www"), []byte("blahgeek"), []byte("com"),
	})
	query_bytes := []byte("\xde\xad\x01\x00\x00\x01\x00\x00\x00\x00\x00\x00" +
		"\x03www\x08blahgeek\x03com\x00\x00\x10\x00\x01")
	if bytes.Compare(query, query_bytes) != 0 {
		t.Errorf("Make DNS Query error: %v\n", query)
	}

	id, labels, err := ParseDNSQuery(query_bytes)
	if err != nil {
		t.Errorf("Parse DNS Query error: %v\n", err)
	}
	if id != 0xDEAD || len(labels) != 3 || bytes.Compare(labels[2], []byte("com")) != 0 {
		t.Error("Parse DNS Query error\n")
	}
}
