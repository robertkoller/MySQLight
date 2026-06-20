package executor

// executeDelete handles DELETE FROM ... WHERE ...
func executeDelete(stmt interface{}) error {
	// TODO: look up the table in the catalog
	// TODO: acquire an exclusive lock on the table
	// TODO: build a TableScan + Filter operator pipeline to find matching rows
	// TODO: for each matching row:
	//   - check if any child table has a FK referencing this row
	//   - apply the FK's ON DELETE action:
	//       RESTRICT → return a foreign key violation error before deleting anything
	//       CASCADE  → recursively delete the child rows first
	//       SET NULL → set the FK column to NULL in all child rows
	//   - write a WAL UPDATE record (before-image of affected pages)
	//   - call btree.Delete(primaryKeyBytes) on the table B+ tree
	//   - for each index on this table: call indexBTree.Delete(indexKeyBytes)
	panic("not implemented")
}
