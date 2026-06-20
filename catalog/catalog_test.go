package catalog

import "testing"

func TestCreateAndGetTable(t *testing.T) {
	// TODO: open a Catalog
	// TODO: CreateTable with a few columns
	// TODO: GetTable and assert the definition matches
	t.Skip("not implemented")
}

func TestDropTable(t *testing.T) {
	// TODO: create a table, drop it, assert GetTable returns an error
	t.Skip("not implemented")
}

func TestCatalogPersistence(t *testing.T) {
	// TODO: create tables and indexes, close the catalog, reopen it
	// TODO: assert all definitions are still present
	t.Skip("not implemented")
}

func TestCreateIndex(t *testing.T) {
	// TODO: create a table, add an index, assert IndexesForTable returns it
	t.Skip("not implemented")
}
