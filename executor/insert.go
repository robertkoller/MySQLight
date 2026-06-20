package executor

// executeInsert handles INSERT INTO ... VALUES ...
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
