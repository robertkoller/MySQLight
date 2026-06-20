package executor

// executeUpdate handles UPDATE ... SET ... WHERE ...
func executeUpdate(stmt interface{}) error {
	// TODO: look up the table in the catalog
	// TODO: acquire an exclusive lock on the table
	// TODO: build a TableScan + Filter operator pipeline to find matching rows
	// TODO: for each matching row:
	//   - evaluate the SET expressions to compute new column values
	//   - validate NOT NULL constraints on updated columns
	//   - if an FK column is changing: verify the new value exists in the parent table
	//   - if this row is referenced by child tables: apply ON UPDATE action to children
	//   - write a WAL UPDATE record (before-image of affected pages)
	//   - delete the old row from the B+ tree (btree.Delete)
	//   - insert the updated row (btree.Insert with new serialised bytes)
	//   - update all affected indexes: remove old key, insert new key
	panic("not implemented")
}
