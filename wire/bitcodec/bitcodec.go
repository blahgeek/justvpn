/*
* @Author: BlahGeek
* @Date:   2015-10-31
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-10-31
 */

package bitcodec

import "reflect"
import "encoding/binary"
import "strconv"
import log "github.com/Sirupsen/logrus"

// encode/decode uint32
type Bitcodec struct {
	field_indices []int
	field_bits    []uint32
	field_masks   []uint32
	remain_bits   uint32

	logger *log.Entry
}

func NewBitcodec(v interface{}) *Bitcodec {
	codec := Bitcodec{}
	codec.logger = log.WithField("logger", "Bitcodec")
	codec.logger.WithField("type", reflect.TypeOf(v)).Debug("New bitcodec")

	var total_bits uint32

	val := reflect.ValueOf(v).Elem()
	for i := 0; i < val.NumField(); i += 1 {
		typeField := val.Type().Field(i)
		tag := typeField.Tag

		if bit_length, err := strconv.Atoi(tag.Get("bits")); err != nil {
			codec.logger.WithField("field", typeField.Name).Debug("Ignore field without bits tag")
			continue
		} else {
			codec.logger.WithFields(log.Fields{
				"field": typeField.Name,
				"Bits":  bit_length,
			}).Debug("Found new field")
			codec.field_indices = append(codec.field_indices, i)
			codec.field_bits = append(codec.field_bits, uint32(bit_length))
			codec.field_masks = append(codec.field_masks, (1<<uint(bit_length))-1)
			total_bits += uint32(bit_length)
		}
	}

	if total_bits > 32 {
		codec.logger.WithField("type", reflect.TypeOf(v)).Panic("Unable to build bitcodec")
		return nil
	}
	codec.remain_bits = 32 - total_bits

	return &codec
}

func (codec *Bitcodec) Encode(v interface{}) uint32 {
	val := reflect.ValueOf(v).Elem()
	var ret uint32
	var shifts uint32 = 32

	for i := 0; i < len(codec.field_indices); i += 1 {
		valueField := val.Field(codec.field_indices[i])
		shifts -= codec.field_bits[i]
		ret |= (uint32(valueField.Uint()) & codec.field_masks[i]) << shifts
	}
	return ret
}

func (codec *Bitcodec) EncodeToBytes(v interface{}) [4]byte {
	var ret [4]byte
	binary.BigEndian.PutUint32(ret[:], codec.Encode(v))
	return ret
}

func (codec *Bitcodec) Decode(v uint32, p interface{}) {
	val := reflect.ValueOf(p).Elem()

	v >>= codec.remain_bits
	for i := len(codec.field_indices) - 1; i >= 0; i -= 1 {
		valueField := val.Field(codec.field_indices[i])
		valueField.SetUint(uint64(v & codec.field_masks[i]))
		v >>= codec.field_bits[i]
	}
}

func (codec *Bitcodec) DecodeFromBytes(v []byte, p interface{}) {
	val := binary.BigEndian.Uint32(v)
	codec.Decode(val, p)
}
