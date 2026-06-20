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

// Open reads the system catalog B+ tree from its reserved page range in the database file,
// decoding each row into a TableDef or IndexDef and loading them into in-memory maps so that
// subsequent lookups do not require disk reads.
func Open() (*Catalog, error) {
	// TODO: read the system catalog B+ tree from its reserved page range
	// TODO: decode each row into a TableDef or IndexDef
	// TODO: populate the in-memory tables and indexes maps
	panic("not implemented")
}

// CreateTable registers a new table in the catalog. It returns an error if a table with
// the same name already exists. Otherwise it allocates a fresh B+ tree root page, sets
// def.RootPageID, persists the definition to the system catalog B+ tree, and adds the
// definition to the in-memory table map.
func (c *Catalog) CreateTable(def *TableDef) error {
	// TODO: return error if a table with that name already exists
	// TODO: allocate a new B+ tree root page via the pager
	// TODO: set def.RootPageID, persist the TableDef to the system catalog B+ tree
	// TODO: add to c.tables
	panic("not implemented")
}

// DropTable removes a table and all its associated indexes from the catalog. It returns
// an error if the table does not exist. On success it removes the table's row from the
// system catalog B+ tree, drops every index belonging to the table, and deletes the
// definition from the in-memory table map.
func (c *Catalog) DropTable(name string) error {
	// TODO: return error if table does not exist
	// TODO: drop all indexes that belong to this table
	// TODO: remove the row from the system catalog B+ tree
	// TODO: delete from c.tables
	panic("not implemented")
}

// GetTable looks up a table by name in the in-memory table map and returns its definition.
// It returns an error if no table with that name exists.
func (c *Catalog) GetTable(name string) (*TableDef, error) {
	// TODO: look up c.tables[name]; return error if not found
	panic("not implemented")
}

// ListTables returns a slice containing the definitions of every table currently registered
// in the catalog.
func (c *Catalog) ListTables() []*TableDef {
	// TODO: return all values in c.tables as a slice
	panic("not implemented")
}

// CreateIndex registers a new index in the catalog. It returns an error if an index with
// the same name already exists. Otherwise it allocates a B+ tree root page, sets
// def.RootPageID, persists the definition to the system catalog, and adds it to the
// in-memory index map.
func (c *Catalog) CreateIndex(def *IndexDef) error {
	// TODO: return error if index name already exists
	// TODO: allocate a new B+ tree root page, set def.RootPageID
	// TODO: persist to the system catalog, add to c.indexes
	panic("not implemented")
}

// DropIndex removes an index from the system catalog B+ tree and the in-memory index map.
// It returns an error if no index with that name exists.
func (c *Catalog) DropIndex(name string) error {
	// TODO: return error if index does not exist
	// TODO: remove from system catalog and c.indexes
	panic("not implemented")
}

// GetIndex looks up an index by name in the in-memory index map and returns its definition.
// It returns an error if no index with that name exists.
func (c *Catalog) GetIndex(name string) (*IndexDef, error) {
	// TODO: look up c.indexes[name]; return error if not found
	panic("not implemented")
}

// IndexesForTable returns all index definitions whose TableName field matches the given table.
// The executor uses this when inserting, updating, or deleting rows to know which indexes
// also need to be updated.
func (c *Catalog) IndexesForTable(tableName string) []*IndexDef {
	// TODO: return all indexes whose TableName == tableName
	panic("not implemented")
}
