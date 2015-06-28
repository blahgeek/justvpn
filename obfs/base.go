/*
* @Author: BlahGeek
* @Date:   2015-06-28
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-06-28
 */

package obfs

import "log"
import "fmt"

type Obfusecator interface {
	// Open obfs, with options (json object)
	Open(options map[string]interface{}) error
	// Close obfs
	Close() error

	// len(obfsed data) - len(plain data)
	GetLengthDelta() int

	Encode([]byte) []byte
	Decode([]byte) ([]byte, error)
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
