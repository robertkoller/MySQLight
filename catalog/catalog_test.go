package catalog_test

import (
	"os"
	"testing"

	"github.com/robertkoller/MySQLight/catalog"
	"github.com/robertkoller/MySQLight/storage"
)

func newTestCatalog(t *testing.T) (*catalog.Catalog, func()) {
	t.Helper()

	file, err := os.CreateTemp("", "catalog_test_*.db")
	if err != nil {
		t.Fatal(err)
	}
	file.Close()

	pager, err := storage.Open(file.Name())
	if err != nil {
		os.Remove(file.Name())
		t.Fatal(err)
	}

	pool := storage.NewBufferPool(pager, 64)

	tree, err := storage.NewBTree(pool, 0)
	if err != nil {
		pager.Close()
		os.Remove(file.Name())
		t.Fatal(err)
	}

	cat, err := catalog.Open(tree)
	if err != nil {
		pager.Close()
		os.Remove(file.Name())
		t.Fatal(err)
	}

	return cat, func() {
		pool.FlushAll()
		pager.Close()
		os.Remove(file.Name())
	}
}

func sampleTable(name string) *catalog.TableDef {
	return &catalog.TableDef{
		Name: name,
		Columns: []catalog.ColumnDef{
			{Name: "id", Type: catalog.TypeInt, PrimaryKey: true, NotNull: true},
			{Name: "email", Type: catalog.TypeText, NotNull: true},
		},
	}
}

func sampleIndex(name, tableName, columnName string, unique bool) *catalog.IndexDef {
	return &catalog.IndexDef{
		Name:       name,
		TableName:  tableName,
		ColumnName: columnName,
		Unique:     unique,
	}
}

func TestCreateTableAndGet(t *testing.T) {
	cat, cleanup := newTestCatalog(t)
	defer cleanup()

	if err := cat.CreateTable(sampleTable("users")); err != nil {
		t.Fatalf("CreateTable: %v", err)
	}

	got, err := cat.GetTable("users")
	if err != nil {
		t.Fatalf("GetTable: %v", err)
	}
	if got.Name != "users" {
		t.Errorf("got name %q, want %q", got.Name, "users")
	}
	if got.RootPageID == 0 {
		t.Error("RootPageID should be non-zero after CreateTable")
	}
}

func TestCreateTableDuplicate(t *testing.T) {
	cat, cleanup := newTestCatalog(t)
	defer cleanup()

	if err := cat.CreateTable(sampleTable("users")); err != nil {
		t.Fatalf("first CreateTable: %v", err)
	}
	if err := cat.CreateTable(sampleTable("users")); err == nil {
		t.Error("expected error on duplicate table, got nil")
	}
}

func TestGetTableNotFound(t *testing.T) {
	cat, cleanup := newTestCatalog(t)
	defer cleanup()

	if _, err := cat.GetTable("nonexistent"); err == nil {
		t.Error("expected error for missing table, got nil")
	}
}

func TestListTables(t *testing.T) {
	cat, cleanup := newTestCatalog(t)
	defer cleanup()

	if tables := cat.ListTables(); len(tables) != 0 {
		t.Errorf("expected 0 tables on empty catalog, got %d", len(tables))
	}

	cat.CreateTable(sampleTable("users"))
	cat.CreateTable(sampleTable("orders"))

	if tables := cat.ListTables(); len(tables) != 2 {
		t.Errorf("expected 2 tables, got %d", len(tables))
	}
}

func TestDropTable(t *testing.T) {
	cat, cleanup := newTestCatalog(t)
	defer cleanup()

	cat.CreateTable(sampleTable("users"))

	if err := cat.DropTable("users"); err != nil {
		t.Fatalf("DropTable: %v", err)
	}
	if _, err := cat.GetTable("users"); err == nil {
		t.Error("expected error after drop, got nil")
	}
}

func TestDropTableNotFound(t *testing.T) {
	cat, cleanup := newTestCatalog(t)
	defer cleanup()

	if err := cat.DropTable("nonexistent"); err == nil {
		t.Error("expected error dropping nonexistent table, got nil")
	}
}

func TestDropTableDropsIndexes(t *testing.T) {
	cat, cleanup := newTestCatalog(t)
	defer cleanup()

	cat.CreateTable(sampleTable("users"))
	cat.CreateIndex(sampleIndex("idx_email", "users", "email", true))
	cat.CreateIndex(sampleIndex("idx_name", "users", "name", false))

	if err := cat.DropTable("users"); err != nil {
		t.Fatalf("DropTable: %v", err)
	}

	if _, err := cat.GetIndex("idx_email"); err == nil {
		t.Error("expected idx_email to be dropped with table")
	}
	if _, err := cat.GetIndex("idx_name"); err == nil {
		t.Error("expected idx_name to be dropped with table")
	}
}

func TestCreateIndexAndGet(t *testing.T) {
	cat, cleanup := newTestCatalog(t)
	defer cleanup()

	cat.CreateTable(sampleTable("users"))
	if err := cat.CreateIndex(sampleIndex("idx_email", "users", "email", true)); err != nil {
		t.Fatalf("CreateIndex: %v", err)
	}

	got, err := cat.GetIndex("idx_email")
	if err != nil {
		t.Fatalf("GetIndex: %v", err)
	}
	if got.Name != "idx_email" {
		t.Errorf("got name %q, want %q", got.Name, "idx_email")
	}
	if !got.Unique {
		t.Error("expected Unique=true")
	}
	if got.RootPageID == 0 {
		t.Error("RootPageID should be non-zero after CreateIndex")
	}
}

func TestCreateIndexDuplicate(t *testing.T) {
	cat, cleanup := newTestCatalog(t)
	defer cleanup()

	cat.CreateTable(sampleTable("users"))
	cat.CreateIndex(sampleIndex("idx_email", "users", "email", true))

	if err := cat.CreateIndex(sampleIndex("idx_email", "users", "email", true)); err == nil {
		t.Error("expected error on duplicate index, got nil")
	}
}

func TestGetIndexNotFound(t *testing.T) {
	cat, cleanup := newTestCatalog(t)
	defer cleanup()

	if _, err := cat.GetIndex("nonexistent"); err == nil {
		t.Error("expected error for missing index, got nil")
	}
}

func TestDropIndex(t *testing.T) {
	cat, cleanup := newTestCatalog(t)
	defer cleanup()

	cat.CreateTable(sampleTable("users"))
	cat.CreateIndex(sampleIndex("idx_email", "users", "email", true))

	if err := cat.DropIndex("idx_email"); err != nil {
		t.Fatalf("DropIndex: %v", err)
	}
	if _, err := cat.GetIndex("idx_email"); err == nil {
		t.Error("expected error after drop, got nil")
	}
}

func TestDropIndexNotFound(t *testing.T) {
	cat, cleanup := newTestCatalog(t)
	defer cleanup()

	if err := cat.DropIndex("nonexistent"); err == nil {
		t.Error("expected error dropping nonexistent index, got nil")
	}
}

func TestDropIndexUpdatesTableIndexes(t *testing.T) {
	cat, cleanup := newTestCatalog(t)
	defer cleanup()

	cat.CreateTable(sampleTable("users"))
	cat.CreateIndex(sampleIndex("idx_email", "users", "email", true))
	cat.CreateIndex(sampleIndex("idx_name", "users", "name", false))

	if err := cat.DropIndex("idx_email"); err != nil {
		t.Fatalf("DropIndex: %v", err)
	}

	indexes, err := cat.IndexesForTable("users")
	if err != nil {
		t.Fatalf("IndexesForTable: %v", err)
	}
	if len(indexes) != 1 {
		t.Errorf("expected 1 index remaining, got %d", len(indexes))
	}
	if indexes[0].Name != "idx_name" {
		t.Errorf("expected idx_name, got %q", indexes[0].Name)
	}
}

func TestIndexesForTable(t *testing.T) {
	cat, cleanup := newTestCatalog(t)
	defer cleanup()

	cat.CreateTable(sampleTable("users"))

	indexes, err := cat.IndexesForTable("users")
	if err != nil {
		t.Fatalf("IndexesForTable on empty table: %v", err)
	}
	if len(indexes) != 0 {
		t.Errorf("expected 0 indexes, got %d", len(indexes))
	}

	cat.CreateIndex(sampleIndex("idx_email", "users", "email", true))
	cat.CreateIndex(sampleIndex("idx_name", "users", "name", false))

	indexes, err = cat.IndexesForTable("users")
	if err != nil {
		t.Fatalf("IndexesForTable with indexes: %v", err)
	}
	if len(indexes) != 2 {
		t.Errorf("expected 2 indexes, got %d", len(indexes))
	}
}

func TestIndexesForTableNotFound(t *testing.T) {
	cat, cleanup := newTestCatalog(t)
	defer cleanup()

	if _, err := cat.IndexesForTable("nonexistent"); err == nil {
		t.Error("expected error for nonexistent table, got nil")
	}
}

func TestOpenLoadsExistingData(t *testing.T) {
	file, err := os.CreateTemp("", "catalog_open_test_*.db")
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	defer os.Remove(file.Name())

	pager, err := storage.Open(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	pool := storage.NewBufferPool(pager, 64)
	defer func() {
		pool.FlushAll()
		pager.Close()
	}()

	tree, err := storage.NewBTree(pool, 0)
	if err != nil {
		t.Fatal(err)
	}

	cat1, err := catalog.Open(tree)
	if err != nil {
		t.Fatal(err)
	}
	cat1.CreateTable(sampleTable("users"))
	cat1.CreateIndex(sampleIndex("idx_email", "users", "email", true))

	// open a second catalog on the same tree — simulates reload on startup
	cat2, err := catalog.Open(tree)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}

	if _, err := cat2.GetTable("users"); err != nil {
		t.Error("users table not found after re-open")
	}
	if _, err := cat2.GetIndex("idx_email"); err != nil {
		t.Error("idx_email not found after re-open")
	}

	indexes, err := cat2.IndexesForTable("users")
	if err != nil {
		t.Fatalf("IndexesForTable after re-open: %v", err)
	}
	if len(indexes) != 1 {
		t.Errorf("expected 1 index after re-open, got %d", len(indexes))
	}
}
