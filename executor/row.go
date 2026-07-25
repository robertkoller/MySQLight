package executor

import (
	"encoding/binary"
	"errors"
	"math"

	"github.com/robertkoller/MySQLight/catalog"
)

func setBit(bitmap []byte, i int, on bool) {
	if on {
		bitmap[i/8] |= 1 << (i % 8)
	} else {
		bitmap[i/8] &^= 1 << (i % 8)
	}
}

func checkBit(bitmap []byte, i int) bool {
	return (bitmap[i/8]>>(i%8))&1 == 1
}

// Row encoding layout (variable-length byte slice):
//   [0 – ceil(n/8)-1]  null bitmap — one bit per column, bit i=1 means column i is NULL
//   [for each non-null column, in column order]
//     TypeInt:   int64  as uint64 big-endian (8 bytes)
//     TypeFloat: float64 as uint64 IEEE 754 bits big-endian (8 bytes)
//     TypeText:  length (uint32 big-endian) + UTF-8 bytes
//     TypeBlob:  length (uint32 big-endian) + raw bytes
//   Null columns contribute nothing beyond their bitmap bit.

// encodeRow serializes a slice of Values into []byte using the row encoding layout above.
// The columns slice provides the type for each value and must be parallel to values.
func encodeRow(values []catalog.Value, columns []catalog.ColumnDef) []byte {
	var output []byte
	bitmap := make([]byte, int(math.Ceil(float64(len(columns))/8.0)))

	for i, value := range values {
		if value.IsNull {
			setBit(bitmap, i, true)
		} else {
			var buffer []byte
			switch columns[i].Type {
			case catalog.TypeInt:
				buffer = make([]byte, 8)
				binary.BigEndian.PutUint64(buffer, uint64(values[i].IntVal))
			case catalog.TypeFloat:
				buffer = make([]byte, 8)
				binary.BigEndian.PutUint64(buffer, math.Float64bits(values[i].FloatVal))
			case catalog.TypeText:
				text := values[i].TextVal
				length := len(text)
				buffer = make([]byte, 4+int(length))
				binary.BigEndian.PutUint32(buffer[0:], uint32(length))
				copy(buffer[4:], text)
			case catalog.TypeBlob:
				blob := values[i].BlobVal
				length := len(blob)
				buffer = make([]byte, 4+int(length))
				binary.BigEndian.PutUint32(buffer[0:], uint32(length))
				copy(buffer[4:], blob)
			}
			output = append(output, buffer...)

		}
	}

	return append(bitmap, output...)
}

// decodeRow deserializes a []byte produced by encodeRow back into a slice of Values,
// using the column definitions to know each field's type and width.
func decodeRow(data []byte, columns []catalog.ColumnDef) ([]catalog.Value, error) {
	bitmapSize := int(math.Ceil(float64(len(columns)) / 8.0))
	if len(data) < bitmapSize {
		return nil, errors.New("row data too short to contain null bitmap")
	}

	bitmap := data[0:bitmapSize]
	current := bitmapSize
	var output []catalog.Value

	for i, column := range columns {
		if checkBit(bitmap, i) {
			output = append(output, catalog.Value{IsNull: true})
			continue
		}

		var value catalog.Value
		switch column.Type {
		case catalog.TypeInt:
			if current+8 > len(data) {
				return nil, errors.New("row data truncated reading int")
			}
			value.IntVal = int64(binary.BigEndian.Uint64(data[current:]))
			current += 8
		case catalog.TypeFloat:
			if current+8 > len(data) {
				return nil, errors.New("row data truncated reading float")
			}
			value.FloatVal = math.Float64frombits(binary.BigEndian.Uint64(data[current:]))
			current += 8
		case catalog.TypeText:
			if current+4 > len(data) {
				return nil, errors.New("row data truncated reading text length")
			}
			length := int(binary.BigEndian.Uint32(data[current:]))
			if current+4+length > len(data) {
				return nil, errors.New("row data truncated reading text bytes")
			}
			value.TextVal = string(data[current+4 : current+4+length])
			current += 4 + length
		case catalog.TypeBlob:
			if current+4 > len(data) {
				return nil, errors.New("row data truncated reading blob length")
			}
			length := int(binary.BigEndian.Uint32(data[current:]))
			if current+4+length > len(data) {
				return nil, errors.New("row data truncated reading blob bytes")
			}
			value.BlobVal = data[current+4 : current+4+length]
			current += 4 + length
		}
		output = append(output, value)
	}

	return output, nil
}
