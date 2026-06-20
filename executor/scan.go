package executor

// TableScan iterates every row in a table's B+ tree leaf chain.
type TableScan struct {
	// TODO: tableDef  — column definitions, needed to deserialise rows
	// TODO: iter      — the BTree Iterator returned by btree.Scan(nil, nil)
}

func NewTableScan(tableName string) *TableScan {
	// TODO: look up the table in the catalog to get its RootPageID and column defs
	// TODO: store a reference; actual scan starts in Open()
	panic("not implemented")
}

func (s *TableScan) Open() error {
	// TODO: acquire a shared lock on the table via the lock manager
	// TODO: call btree.Scan(nil, nil) to get an iterator over all leaves
	panic("not implemented")
}

func (s *TableScan) Next() (Row, error) {
	// TODO: call iter.Next() to get the next raw key-value bytes
	// TODO: deserialise the value bytes into a Row using the column definitions
	// TODO: propagate io.EOF when the iterator is exhausted
	panic("not implemented")
}

func (s *TableScan) Close() error {
	// TODO: iter.Close()
	// TODO: release the shared lock
	panic("not implemented")
}

// IndexScan traverses a B+ tree index and fetches matching rows by key.
type IndexScan struct {
	// TODO: indexDef  — needed to locate the index B+ tree
	// TODO: tableDef  — needed to deserialise the fetched rows
	// TODO: startKey, endKey []byte
	// TODO: iter      — BTree Iterator over the index range
}

func NewIndexScan(indexName string, startKey, endKey []byte) *IndexScan {
	// TODO: look up the index and its parent table in the catalog
	panic("not implemented")
}

func (s *IndexScan) Open() error {
	// TODO: acquire a shared lock on the table
	// TODO: call indexBTree.Scan(startKey, endKey) to get the iterator
	panic("not implemented")
}

func (s *IndexScan) Next() (Row, error) {
	// TODO: call iter.Next() to get the next index key → primary key mapping
	// TODO: use the primary key to fetch the full row from the table B+ tree
	// TODO: deserialise and return the row
	panic("not implemented")
}

func (s *IndexScan) Close() error {
	// TODO: iter.Close(), release lock
	panic("not implemented")
}
