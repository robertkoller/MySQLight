package executor

// executeInsert handles INSERT INTO ... VALUES ... by looking up the table definition,
// acquiring an exclusive lock, and inserting each value row. For each row it evaluates
// expressions, validates NOT NULL and DEFAULT constraints, verifies any foreign key
// references in parent tables, serialises the row to bytes, and inserts into both the
// table B+ tree and all associated index B+ trees. WAL UPDATE records are written before
// any page is modified, following the WAL protocol.
func executeInsert(stmt interface{}) error {
	// TODO: look up the table in the catalog (get TableDef + RootPageID)
	// TODO: acquire an exclusive lock on the table via the lock manager
	// TODO: for each value row in stmt.Values:
	//   - evaluate all expressions to get concrete values
	//   - validate NOT NULL constraints
	//   - validate DEFAULT values for omitted columns
	//   - for each FK column: verify the referenced key exists in the parent table
	//   - serialise the row to bytes (null bitmap + fixed/variable fields)
	//   - call btree.Insert(primaryKeyBytes, rowBytes)
	//   - for each index on this table: call indexBTree.Insert(indexKeyBytes, primaryKeyBytes)
	// TODO: write WAL UPDATE records before modifying any page (WAL protocol)
	panic("not implemented")
}
