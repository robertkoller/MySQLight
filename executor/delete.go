package executor

// executeDelete handles DELETE FROM ... WHERE ... by looking up the table, acquiring an
// exclusive lock, and scanning with a filter to find matching rows. For each row it checks
// whether any child table holds a foreign key reference and applies the configured ON DELETE
// action: RESTRICT returns an error before anything is deleted, CASCADE deletes child rows
// recursively, and SET NULL nulls the FK column in all referencing child rows. After FK
// handling, a WAL before-image record is written and then the row is deleted from the B+
// tree and all associated indexes.
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
