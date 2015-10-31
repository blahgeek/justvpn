/*
* @Author: BlahGeek
* @Date:   2015-10-31
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-10-31
 */

package bitcodec

import "testing"
import log "github.com/Sirupsen/logrus"

type S0 struct {
	A uint   `bits:"1"`
	B uint16 `bits:"7"`
	C byte   `bits:"8"`
}

func TestBitcodec(t *testing.T) {
	log.SetLevel(log.DebugLevel)

	codec := NewBitcodec(&S0{})
	if codec == nil {
		t.Fatal("Unable to build bitcodec")
	}

	tests := []struct {
		x   S0
		val uint32
	}{
		{S0{}, 0},
		{S0{A: 1}, 0x80000000},
		{S0{A: 0, C: 255}, 0x00ff0000},
	}

	for _, test := range tests {
		ret := codec.Encode(&test.x)
		if ret != test.val {
			t.Errorf("Encode fail: %v != %#x, %#x", test.x, test.val, ret)
		}
	}

	for _, test := range tests {
		var ret S0
		codec.Decode(test.val, &ret)
		if ret != test.x {
			t.Errorf("Decode fail: %v != %#x, %v", test.x, test.val, ret)
		}
	}
}
