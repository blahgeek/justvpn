/*
* @Author: BlahGeek
* @Date:   2015-06-28
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-06-28
 */

package obfs

import "fmt"

type XorObfusecator struct {
	key []byte
}

func (xor *XorObfusecator) Open(options map[string]interface{}) error {
	if field := options["key"]; field == nil {
		return fmt.Errorf("`key` not found in options")
	} else {
		key_str := field.(string)
		xor.key = make([]byte, len(key_str))
		copy(xor.key, key_str)
	}
	return nil
}

func (xor *XorObfusecator) Close() error { return nil }

func (xor *XorObfusecator) GetLengthDelta() int { return 0 }

func (xor *XorObfusecator) Encode(buf []byte) []byte {
	ret := make([]byte, len(buf))
	for i := 0; i < len(buf); i += 1 {
		c := xor.key[i%len(xor.key)]
		ret[i] = c ^ buf[i]
	}
	return ret
}

// For xor, decoding is same as encoding, error is always nil
func (xor *XorObfusecator) Decode(buf []byte) ([]byte, error) {
	return xor.Encode(buf), nil
}
