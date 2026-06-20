package catalog

type DataType uint8

const (
	TypeInt   DataType = iota // 8-byte signed integer
	TypeFloat DataType = iota // 8-byte IEEE 754 float
	TypeText  DataType = iota // variable-length UTF-8 string
	TypeBlob  DataType = iota // variable-length raw bytes
)

// Value holds a single typed cell value. Exactly one field is set.
type Value struct {
	// TODO: IsNull bool
	// TODO: IntVal   int64
	// TODO: FloatVal float64
	// TODO: TextVal  string
	// TODO: BlobVal  []byte
}

type ColumnDef struct {
	Name       string
	Type       DataType
	PrimaryKey bool
	NotNull    bool
	Default    *Value // nil means no default
}

type FKAction uint8

const (
	FKRestrict FKAction = iota
	FKCascade  FKAction = iota
	FKSetNull  FKAction = iota
)

type ForeignKeyDef struct {
	ColumnName string
	RefTable   string
	RefColumn  string
	OnDelete   FKAction
	OnUpdate   FKAction
}
