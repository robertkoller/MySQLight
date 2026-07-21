package catalog

import (
	"encoding/binary"
	"errors"
	"math"
)

// Claude cooked with this visualization

// TableDef encoding layout (variable-length byte slice):
//   [0]        kind byte: 0x01 = TableDef
//   [1–4]      name length (uint32)
//   [5–...]    name bytes
//   [...]      column count (uint16)
//   [for each column]
//     [...]    name length (uint32) + name bytes
//     [...]    type (uint8): 0=INT 1=FLOAT 2=TEXT 3=BLOB
//     [...]    flags (uint8): bit0=PrimaryKey bit1=NotNull bit2=hasDefault
//     [if hasDefault]
//       [...]  isNull (uint8): 1 if null default
//       [if not null]
//         INT:   value as int64  (8 bytes)
//         FLOAT: value as uint64 (8 bytes, IEEE 754 bits)
//         TEXT:  length (uint32) + bytes
//         BLOB:  length (uint32) + bytes
//   [...]      foreign key count (uint16)
//   [for each FK]
//     [...]    columnName length (uint32) + bytes
//     [...]    refTable   length (uint32) + bytes
//     [...]    refColumn  length (uint32) + bytes
//     [...]    OnDelete action (uint8): 0=RESTRICT 1=CASCADE 2=SET NULL
//     [...]    OnUpdate action (uint8)
//   [...]      RootPageID (uint32)

// encodeTableDef serializes a TableDef into a byte slice using the catalog encoding format:
// a kind byte, length-prefixed strings, fixed-width integers, per-column flags, and a null
// bitmap for default values. The resulting bytes are stored as a value in the catalog B+ tree.
func encodeTableDef(def *TableDef) []byte {
	var buffer []byte

	buffer = append(buffer, 0x01)

	nameBytes := []byte(def.Name)
	var nameLenBuf [4]byte
	binary.BigEndian.PutUint32(nameLenBuf[:], uint32(len(nameBytes)))
	buffer = append(buffer, nameLenBuf[:]...)
	buffer = append(buffer, nameBytes...)

	var colCountBuf [2]byte
	binary.BigEndian.PutUint16(colCountBuf[:], uint16(len(def.Columns)))
	buffer = append(buffer, colCountBuf[:]...)

	for _, columns := range def.Columns {
		colNameBytes := []byte(columns.Name)
		var colNameLenBuf [4]byte
		binary.BigEndian.PutUint32(colNameLenBuf[:], uint32(len(colNameBytes)))
		buffer = append(buffer, colNameLenBuf[:]...)
		buffer = append(buffer, colNameBytes...)

		switch columns.Type {
		case TypeInt:
			buffer = append(buffer, 0)
		case TypeFloat:
			buffer = append(buffer, 1)
		case TypeText:
			buffer = append(buffer, 2)
		case TypeBlob:
			buffer = append(buffer, 3)
		}

		// we want these flags to be as bits and go is annoying and only lets us work with bytes so we gotta do this shenanigans
		var flags byte
		if columns.PrimaryKey {
			flags |= 0b00000001
		}
		if columns.NotNull {
			flags |= 0b00000010
		}
		if columns.Default != nil {
			flags |= 0b00000100
		}
		buffer = append(buffer, flags)

		if columns.Default != nil {
			if columns.Default.IsNull {
				buffer = append(buffer, 1)
			} else {
				buffer = append(buffer, 0)
				switch columns.Type {
				case TypeInt:
					var valueBuf [8]byte
					binary.BigEndian.PutUint64(valueBuf[:], uint64(columns.Default.IntVal))
					buffer = append(buffer, valueBuf[:]...)
				case TypeFloat:
					var valueBuf [8]byte
					binary.BigEndian.PutUint64(valueBuf[:], math.Float64bits(columns.Default.FloatVal))
					buffer = append(buffer, valueBuf[:]...)
				case TypeText:
					textBytes := []byte(columns.Default.TextVal)
					var textLenBuf [4]byte
					binary.BigEndian.PutUint32(textLenBuf[:], uint32(len(textBytes)))
					buffer = append(buffer, textLenBuf[:]...)
					buffer = append(buffer, textBytes...)
				case TypeBlob:
					var blobLenBuf [4]byte
					binary.BigEndian.PutUint32(blobLenBuf[:], uint32(len(columns.Default.BlobVal)))
					buffer = append(buffer, blobLenBuf[:]...)
					buffer = append(buffer, columns.Default.BlobVal...)
				}
			}
		}
	}

	// Foreign keys time bbaby
	var fkCountBuf [2]byte
	binary.BigEndian.PutUint16(fkCountBuf[:], uint16(len(def.ForeignKeys)))
	buffer = append(buffer, fkCountBuf[:]...)

	for _, foreignKey := range def.ForeignKeys {
		for _, str := range []string{foreignKey.ColumnName, foreignKey.RefTable, foreignKey.RefColumn} {
			strBytes := []byte(str)
			var strLenBuf [4]byte
			binary.BigEndian.PutUint32(strLenBuf[:], uint32(len(strBytes)))
			buffer = append(buffer, strLenBuf[:]...)
			buffer = append(buffer, strBytes...)
		}

		switch foreignKey.OnDelete {
		case FKRestrict:
			buffer = append(buffer, 0)
		case FKCascade:
			buffer = append(buffer, 1)
		case FKSetNull:
			buffer = append(buffer, 2)
		}

		switch foreignKey.OnUpdate {
		case FKRestrict:
			buffer = append(buffer, 0)
		case FKCascade:
			buffer = append(buffer, 1)
		case FKSetNull:
			buffer = append(buffer, 2)
		}
	}

	var rootPageIDBuf [4]byte
	binary.BigEndian.PutUint32(rootPageIDBuf[:], def.RootPageID)
	buffer = append(buffer, rootPageIDBuf[:]...)

	return buffer
}

// decodeTableDef deserializes a byte slice produced by encodeTableDef back into a TableDef.
func decodeTableDef(data []byte) (*TableDef, error) {
	var name string
	var columns []ColumnDef
	var keys []ForeignKeyDef
	var rootPageID uint32

	if data[0] != 0x01 {
		return &TableDef{}, errors.New("does not contain TableDef kind byte")
	}

	lenName := binary.BigEndian.Uint32(data[1:])
	current := 5
	name = string(data[current : current+int(lenName)])
	current += int(lenName)

	numColumns := int(binary.BigEndian.Uint16(data[current:]))
	current += 2

	for i := 0; i < numColumns; i++ {
		var column ColumnDef

		lenName := binary.BigEndian.Uint32(data[current:])
		column.Name = string(data[current+4 : current+4+int(lenName)])
		current += 4 + int(lenName)

		switch data[current] {
		case 0:
			column.Type = TypeInt
		case 1:
			column.Type = TypeFloat
		case 2:
			column.Type = TypeText
		case 3:
			column.Type = TypeBlob
		}
		current++

		column.PrimaryKey = GetBit(data[current], 0)
		column.NotNull = GetBit(data[current], 1)
		hasDefault := GetBit(data[current], 2)
		current++

		if !hasDefault {
			column.Default = nil
		} else {
			if data[current] == 0 { // not null
				current++
				column.Default = &Value{}
				column.Default.IsNull = false

				switch column.Type {
				case TypeInt:
					column.Default.IntVal = int64(binary.BigEndian.Uint64(data[current:]))
					current += 8
				case TypeFloat:
					bits := binary.BigEndian.Uint64(data[current:])
					column.Default.FloatVal = math.Float64frombits(bits)
					current += 8
				case TypeText:
					length := binary.BigEndian.Uint32(data[current:])
					column.Default.TextVal = string(data[current+4 : current+4+int(length)])
					current += 4 + int(length)
				case TypeBlob:
					length := binary.BigEndian.Uint32(data[current:])
					column.Default.BlobVal = data[current+4 : current+4+int(length)]
					current += 4 + int(length)
				}
			} else {
				current++
				column.Default = &Value{IsNull: true}
			}
		}

		columns = append(columns, column)

	}

	fkeyCount := binary.BigEndian.Uint16(data[current:])
	current += 2

	for i := 0; i < int(fkeyCount); i++ {
		var key ForeignKeyDef

		length := binary.BigEndian.Uint32(data[current:])
		key.ColumnName = string(data[current+4 : current+4+int(length)])
		current += 4 + int(length)

		length = binary.BigEndian.Uint32(data[current:])
		key.RefTable = string(data[current+4 : current+4+int(length)])
		current += 4 + int(length)

		length = binary.BigEndian.Uint32(data[current:])
		key.RefColumn = string(data[current+4 : current+4+int(length)])
		current += 4 + int(length)

		switch data[current] {
		case 0:
			key.OnDelete = FKRestrict
		case 1:
			key.OnDelete = FKCascade
		case 2:
			key.OnDelete = FKSetNull
		}
		current++

		switch data[current] {
		case 0:
			key.OnUpdate = FKRestrict
		case 1:
			key.OnUpdate = FKCascade
		case 2:
			key.OnUpdate = FKSetNull
		}
		current++

		keys = append(keys, key)
	}

	rootPageID = binary.BigEndian.Uint32(data[current:])

	return &TableDef{
		Name:        name,
		Columns:     columns,
		ForeignKeys: keys,
		RootPageID:  rootPageID,
	}, nil
}

// IndexDef encoding layout (variable-length byte slice):
//   [0]        kind byte: 0x02 = IndexDef
//   [1–4]      name length (uint32) + name bytes
//   [...]      tableName  length (uint32) + bytes
//   [...]      columnName length (uint32) + bytes
//   [...]      unique (uint8): 0 or 1
//   [...]      RootPageID (uint32)

// encodeIndexDef serializes an IndexDef into a byte slice.
func encodeIndexDef(def *IndexDef) []byte {
	var buffer []byte

	buffer = append(buffer, 0x02)

	for _, str := range []string{def.Name, def.TableName, def.ColumnName} {
		strBytes := []byte(str)
		var strLenBuf [4]byte
		binary.BigEndian.PutUint32(strLenBuf[:], uint32(len(strBytes)))
		buffer = append(buffer, strLenBuf[:]...)
		buffer = append(buffer, strBytes...)
	}

	switch def.Unique {
	case false:
		buffer = append(buffer, 0)
	case true:
		buffer = append(buffer, 1)
	}

	var rootBuf [4]byte
	binary.BigEndian.PutUint32(rootBuf[:], def.RootPageID)
	buffer = append(buffer, rootBuf[:]...)

	return buffer
}

// decodeIndexDef deserializes a byte slice produced by encodeIndexDef back into an IndexDef.
func decodeIndexDef(data []byte) (*IndexDef, error) {

	var name string
	var table string
	var column string
	var unique bool
	var rootPageID uint32

	if data[0] != 0x02 {
		return &IndexDef{}, errors.New("does not contain IndexDef kind byte")
	}

	length := binary.BigEndian.Uint32(data[1:])
	current := 5
	name = string(data[current : current+int(length)])
	current += int(length)

	length = binary.BigEndian.Uint32(data[current:])
	current += 4
	table = string(data[current : current+int(length)])
	current += int(length)

	length = binary.BigEndian.Uint32(data[current:])
	current += 4
	column = string(data[current : current+int(length)])
	current += int(length)

	switch data[current] {
	case 0:
		unique = false
	case 1:
		unique = true
	}
	current++

	rootPageID = binary.BigEndian.Uint32(data[current:])

	return &IndexDef{
		Name:       name,
		TableName:  table,
		ColumnName: column,
		Unique:     unique,
		RootPageID: rootPageID,
	}, nil
}

// returns the bit at the position where we start from the right
func GetBit(b byte, position int) bool {
	bit := (b >> position) & 1

	if bit == 0 {
		return false
	} else {
		return true
	}
}
