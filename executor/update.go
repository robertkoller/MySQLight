package executor

// executeUpdate handles UPDATE ... SET ... WHERE ... by looking up the table, acquiring an
// exclusive lock, and scanning with a filter to find matching rows. For each matching row
// it evaluates the SET expressions, validates NOT NULL constraints, checks foreign key rules
// in both directions (verifying new values exist in parent tables and applying ON UPDATE
// actions to child rows), writes a WAL before-image record, deletes the old B+ tree entry,
// inserts the updated entry, and updates all affected indexes.
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
