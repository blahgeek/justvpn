/*
* @Author: BlahGeek
* @Date:   2015-06-28
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-08-14
 */

package obfs

import "encoding/json"
import log "github.com/Sirupsen/logrus"

type XorObfusecatorOptions struct {
	Key string `json:"key"`
}

type XorObfusecator struct {
	options XorObfusecatorOptions
	max_len int

	logger *log.Entry
}

func (xor *XorObfusecator) Open(options json.RawMessage, max_obfsed_len int) error {
	xor.logger = log.WithField("logger", "XorObfusecator")
	if err := json.Unmarshal(options, &xor.options); err != nil {
		return err
	}
	xor.max_len = max_obfsed_len
	xor.logger.WithFields(log.Fields{
		"key":     xor.options.Key,
		"max_len": max_obfsed_len,
	}).Info("XOR Obfusecator init done")
	return nil
}

func (xor *XorObfusecator) Close() error { return nil }

func (xor *XorObfusecator) GetMaxPlainLength() int { return xor.max_len }

func (xor *XorObfusecator) Encode(src, dst []byte) int {
	for i := 0; i < len(src); i += 1 {
		c := xor.options.Key[i%len(xor.options.Key)]
		dst[i] = c ^ src[i]
	}
	return len(src)
}

// For xor, decoding is same as encoding, error is always nil
func (xor *XorObfusecator) Decode(src, dst []byte) (int, error) {
	return xor.Encode(src, dst), nil
}
