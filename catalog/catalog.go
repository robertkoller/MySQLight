package catalog

import (
	"errors"
	"io"

	"github.com/robertkoller/MySQLight/storage"
)

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
	tables       map[string]*TableDef
	indexes      map[string]*IndexDef // keyed by index name
	tableIndexes map[string][]string  // table name → index names for O(1) DropTable
	tree         *storage.BTree       // reference to the storage layer for persistence
}

// Open reads the system catalog B+ tree from its reserved page range in the database file,
// decoding each row into a TableDef or IndexDef and loading them into in-memory maps so that
// subsequent lookups do not require disk reads.
func Open(tree *storage.BTree) (*Catalog, error) {
	tables := make(map[string]*TableDef)
	indexes := make(map[string]*IndexDef)
	tableIndexes := make(map[string][]string)

	it, err := tree.Scan(nil, nil)
	if err != nil {
		return nil, err
	}

	defer it.Close()

	for {
		key, value, err := it.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch value[0] {
		case 0x01:
			table, err := decodeTableDef(value)
			if err != nil {
				return nil, err
			}
			tables[string(key)] = table
		case 0x02:
			index, err := decodeIndexDef(value)
			if err != nil {
				return nil, err
			}
			indexes[string(key)] = index
			tableIndexes[index.TableName] = append(tableIndexes[index.TableName], string(key))
		}
	}

	return &Catalog{
		tables:       tables,
		indexes:      indexes,
		tableIndexes: tableIndexes,
		tree:         tree,
	}, nil
}

// CreateTable registers a new table in the catalog. It returns an error if a table with
// the same name already exists. Otherwise it allocates a fresh B+ tree root page, sets
// def.RootPageID, persists the definition to the system catalog B+ tree, and adds the
// definition to the in-memory table map.
func (c *Catalog) CreateTable(def *TableDef) error {
	if _, exists := c.tables[def.Name]; exists {
		return errors.New("table with that name already exists")
	}

	pageID, err := c.tree.AllocateNewRoot()
	if err != nil {
		return err
	}
	def.RootPageID = pageID

	encoded := encodeTableDef(def)
	if err := c.tree.Insert([]byte(def.Name), encoded); err != nil {
		return err
	}

	c.tables[def.Name] = def
	c.tableIndexes[def.Name] = []string{}
	return nil
}

// DropTable removes a table and all its associated indexes from the catalog. It returns
// an error if the table does not exist. On success it removes the table's row from the
// system catalog B+ tree, drops every index belonging to the table, and deletes the
// definition from the in-memory table map.
func (c *Catalog) DropTable(name string) error {
	if _, err := c.GetTable(name); err != nil {
		return err
	}

	for _, indexName := range c.tableIndexes[name] {
		if err := c.tree.Delete([]byte(indexName)); err != nil {
			return err
		}
		delete(c.indexes, indexName)
	}
	delete(c.tableIndexes, name)

	if err := c.tree.Delete([]byte(name)); err != nil {
		return err
	}
	delete(c.tables, name)
	return nil
}

// GetTable looks up a table by name in the in-memory table map and returns its definition.
// It returns an error if no table with that name exists.
func (c *Catalog) GetTable(name string) (*TableDef, error) {
	table, exists := c.tables[name]
	if !exists {
		return &TableDef{}, errors.New("Table not found.")
	}
	return table, nil
}

// ListTables returns a slice containing the definitions of every table currently registered
// in the catalog.
func (c *Catalog) ListTables() []*TableDef {
	values := make([]*TableDef, 0, len(c.tables))

	for _, value := range c.tables {
		values = append(values, value)
	}

	return values
}

// CreateIndex registers a new index in the catalog. It returns an error if an index with
// the same name already exists. Otherwise it allocates a B+ tree root page, sets
// def.RootPageID, persists the definition to the system catalog, and adds it to the
// in-memory index map.
func (c *Catalog) CreateIndex(def *IndexDef) error {
	_, exists := c.indexes[def.Name]
	if exists {
		return errors.New("Index already exists")
	}

	pageID, err := c.tree.AllocateNewRoot()
	if err != nil {
		return err
	}
	def.RootPageID = pageID

	encoded := encodeIndexDef(def)
	if err := c.tree.Insert([]byte(def.Name), encoded); err != nil {
		return err
	}

	c.indexes[def.Name] = def
	c.tableIndexes[def.TableName] = append(c.tableIndexes[def.TableName], def.Name)
	return nil
}

// DropIndex removes an index from the system catalog B+ tree and the in-memory index map.
// It returns an error if no index with that name exists.
func (c *Catalog) DropIndex(name string) error {

	index, err := c.GetIndex(name)
	if err != nil {
		return err
	}
	if err := c.tree.Delete([]byte(name)); err != nil {
		return err
	}
	delete(c.indexes, name)
	for idx, indexed := range c.tableIndexes[index.TableName] {
		if indexed == name {
			c.tableIndexes[index.TableName] = append(c.tableIndexes[index.TableName][:idx], c.tableIndexes[index.TableName][idx+1:]...)
			break
		}
	}
	return nil
}

// GetIndex looks up an index by name in the in-memory index map and returns its definition.
// It returns an error if no index with that name exists.
func (c *Catalog) GetIndex(name string) (*IndexDef, error) {
	index, exists := c.indexes[name]
	if !exists {
		return nil, errors.New("Index does not exist in map")
	}

	return index, nil
}

// Returns the indexesdef for a given table
func (c *Catalog) IndexesForTable(tableName string) ([]*IndexDef, error) {
	var output []*IndexDef
	indexes, exists := c.tableIndexes[tableName]
	if !exists {
		return nil, errors.New("Table does not exist")
	}

	for _, index := range indexes {
		indexDef, exists := c.indexes[index]
		if !exists {
			return nil, errors.New("catalog integrity error: index not found")
		}

		output = append(output, indexDef)

	}
	return output, nil
}
