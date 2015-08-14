/*
* @Author: BlahGeek
* @Date:   2015-06-28
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-08-14
 */

package obfs

import log "github.com/Sirupsen/logrus"
import "encoding/json"
import "fmt"

type Obfusecator interface {
	// Open obfs, with options (json object)
	Open(options json.RawMessage, max_obfsed_len int) error
	// Close obfs
	Close() error

	// Return max length of plain data (given max_obfsed_len input data)
	GetMaxPlainLength() int

	// Encode src to dst, return length of dst
	// len(dst) would be at least len(src) + GetMaxOverhead()
	Encode(src, dst []byte) int
	// Decode src to dst, return length of dst
	// len(dst) would be at least len(src) - GetMaxOverhead()
	Decode(src, dst []byte) (int, error)
}

func New(name string, options json.RawMessage, max_obfsed_len int) (Obfusecator, error) {
	var ret Obfusecator
	err := fmt.Errorf("No obfusecator found: %v", name)
	log.WithField("name", name).Info("Allocating new obfusecator")

	switch name {
	case "xor":
		ret = &XorObfusecator{}
		err = ret.Open(options, max_obfsed_len)
	default:
	}

	return ret, err
}
