package catalog

type TableDef struct {
	Name        string
	Columns     []ColumnDef
	ForeignKeys []ForeignKeyDef
	RootPageID  uint32 // root of this table's B+ tree in the storage layer
}

type IndexDef struct {
	Name       string
	TableName  string
	ColumnName string
	Unique     bool
	RootPageID uint32
}

// Catalog owns all table and index metadata. It is itself persisted in a
// reserved B+ tree at a known page range in the database file.
type Catalog struct {
	// TODO: tables  map[string]*TableDef
	// TODO: indexes map[string]*IndexDef  — keyed by index name
	// TODO: pager   reference to the storage layer for persistence
}

func Open() (*Catalog, error) {
	// TODO: read the system catalog B+ tree from its reserved page range
	// TODO: decode each row into a TableDef or IndexDef
	// TODO: populate the in-memory tables and indexes maps
	panic("not implemented")
}

func (c *Catalog) CreateTable(def *TableDef) error {
	// TODO: return error if a table with that name already exists
	// TODO: allocate a new B+ tree root page via the pager
	// TODO: set def.RootPageID, persist the TableDef to the system catalog B+ tree
	// TODO: add to c.tables
	panic("not implemented")
}

func (c *Catalog) DropTable(name string) error {
	// TODO: return error if table does not exist
	// TODO: drop all indexes that belong to this table
	// TODO: remove the row from the system catalog B+ tree
	// TODO: delete from c.tables
	panic("not implemented")
}

func (c *Catalog) GetTable(name string) (*TableDef, error) {
	// TODO: look up c.tables[name]; return error if not found
	panic("not implemented")
}

func (c *Catalog) ListTables() []*TableDef {
	// TODO: return all values in c.tables as a slice
	panic("not implemented")
}

func (c *Catalog) CreateIndex(def *IndexDef) error {
	// TODO: return error if index name already exists
	// TODO: allocate a new B+ tree root page, set def.RootPageID
	// TODO: persist to the system catalog, add to c.indexes
	panic("not implemented")
}

func (c *Catalog) DropIndex(name string) error {
	// TODO: return error if index does not exist
	// TODO: remove from system catalog and c.indexes
	panic("not implemented")
}

func (c *Catalog) GetIndex(name string) (*IndexDef, error) {
	// TODO: look up c.indexes[name]; return error if not found
	panic("not implemented")
}

func (c *Catalog) IndexesForTable(tableName string) []*IndexDef {
	// TODO: return all indexes whose TableName == tableName
	panic("not implemented")
}
