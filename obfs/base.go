/*
* @Author: BlahGeek
* @Date:   2015-06-28
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-06-29
 */

package obfs

import "log"
import "fmt"

type Obfusecator interface {
	// Open obfs, with options (json object)
	Open(options map[string]interface{}) error
	// Close obfs
	Close() error

	// max of `len(obfsed data) - len(plain data)`
	GetMaxOverhead() int

	// Encode src to dst, return length of dst
	// len(dst) would be at least len(src) + GetMaxOverhead()
	Encode(src, dst []byte) int
	// Decode src to dst, return length of dst
	// len(dst) would be at least len(src) - GetMaxOverhead()
	Decode(src, dst []byte) (int, error)
}

func New(name string, options map[string]interface{}) (Obfusecator, error) {
	var ret Obfusecator
	err := fmt.Errorf("No obfusecator found: %v", name)

	switch name {
	case "xor":
		log.Printf("New obfusecator: %v", name)
		ret = &XorObfusecator{}
		err = ret.Open(options)
	default:
	}

	return ret, err
}
