# MySQLight Catalog

Phase 3 of MySQLight. The catalog is the schema registry — it knows every table
and every index that exists in the database, what columns they have, and where
their data lives on disk (root page IDs). It is the bridge between the SQL layer
and the storage layer.

---

## What the catalog does

Every time a query runs, the executor asks the catalog: "does this table exist,
and what are its columns?" When a new table or index is created, the catalog
allocates a fresh B+ tree root page and records the definition. When a table is
dropped, the catalog removes it and all its indexes atomically (in-memory).

The catalog persists all this information in its own reserved B+ tree, which lives
at a known root page ID stored in the database file header (offset 15). On startup,
`Open` scans that tree and loads everything into in-memory maps so subsequent
lookups need no disk access.

---

## Layering

```
        ┌─────────────────────────────┐
        │         Executor            │   (Phase 4)
        └──────────────┬──────────────┘
                       │ GetTable / GetIndex / IndexesForTable
        ┌──────────────▼──────────────┐
        │           Catalog           │   catalog.go
        │  tables map / indexes map   │
        │  tableIndexes map           │
        └──────────────┬──────────────┘
                       │ Insert / Get / Delete / Scan
        ┌──────────────▼──────────────┐
        │      Catalog B+ Tree        │   storage.BTree
        │  key = name   value = bytes │
        └─────────────────────────────┘
```

---

## Schema types (`schema.go`)

### `TableDef`
```go
type TableDef struct {
    Name        string
    Columns     []ColumnDef
    ForeignKeys []ForeignKeyDef
    RootPageID  uint32  // root page of this table's data B+ tree
}
```

### `ColumnDef`
```go
type ColumnDef struct {
    Name       string
    Type       DataType  // TypeInt | TypeFloat | TypeText | TypeBlob
    PrimaryKey bool
    NotNull    bool
    Default    *Value    // nil = no default
}
```

### `Value`
```go
type Value struct {
    IsNull   bool
    IntVal   int64
    FloatVal float64
    TextVal  string
    BlobVal  []byte
}
```
Exactly one field is meaningful; which one is determined by the column's `DataType`.

### `IndexDef`
```go
type IndexDef struct {
    Name       string
    TableName  string
    ColumnName string
    Unique     bool
    RootPageID uint32
}
```

### `ForeignKeyDef`
```go
type ForeignKeyDef struct {
    ColumnName string
    RefTable   string
    RefColumn  string
    OnDelete   FKAction  // FKRestrict | FKCascade | FKSetNull
    OnUpdate   FKAction
}
```

---

## In-memory structure (`catalog.go`)

```go
type Catalog struct {
    tables       map[string]*TableDef
    indexes      map[string]*IndexDef  // keyed by index name
    tableIndexes map[string][]string   // table name → []index name
    tree         *storage.BTree
}
```

`tableIndexes` is a reverse index maintained alongside `indexes` so that
`DropTable` can delete all associated indexes in O(k) — k being the number of
indexes on the table — rather than scanning every index.

---

## Binary encoding (`encoding.go`)

Definitions are serialized to `[]byte` before being stored as values in the
catalog B+ tree. The first byte is a kind discriminator: `0x01` = `TableDef`,
`0x02` = `IndexDef`.

### TableDef layout

```
[0]        kind byte: 0x01
[1–4]      name length (uint32, big-endian)
[5–...]    name bytes
[...]      column count (uint16)
[for each column]
  [...]    name length (uint32) + name bytes
  [...]    type (uint8): 0=INT 1=FLOAT 2=TEXT 3=BLOB
  [...]    flags (uint8): bit0=PrimaryKey  bit1=NotNull  bit2=hasDefault
  [if hasDefault]
    [...]  isNull (uint8): 1 if null default, 0 otherwise
    [if not null]
      INT:   int64  as uint64 big-endian (8 bytes)
      FLOAT: float64 as uint64 IEEE 754 bits big-endian (8 bytes)
      TEXT:  length (uint32) + bytes
      BLOB:  length (uint32) + bytes
[...]      foreign key count (uint16)
[for each FK]
  [...]    columnName, refTable, refColumn — each as length (uint32) + bytes
  [...]    OnDelete action (uint8): 0=RESTRICT 1=CASCADE 2=SET NULL
  [...]    OnUpdate action (uint8)
[...]      RootPageID (uint32)
```

### IndexDef layout

```
[0]        kind byte: 0x02
[...]      name, tableName, columnName — each as length (uint32) + bytes
[...]      unique (uint8): 0 or 1
[...]      RootPageID (uint32)
```

All multi-byte integers are big-endian (`encoding/binary.BigEndian`).

---

## API

```go
Open(tree *storage.BTree) (*Catalog, error)
```
Scans the catalog B+ tree from start to end, decoding each entry into a
`TableDef` or `IndexDef` (based on kind byte) and loading them into the in-memory
maps. Returns an empty but valid catalog if the tree is empty.

```go
(c *Catalog) CreateTable(def *TableDef) error
```
Checks for duplicates, allocates a new leaf page as the table's data tree root
(`AllocateNewRoot`), encodes the def, inserts into the catalog tree, and adds to
`tables` and `tableIndexes`.

```go
(c *Catalog) DropTable(name string) error
```
Deletes all indexes belonging to the table (using `tableIndexes` for O(k) lookup),
removes the table row from the catalog tree, cleans up both maps.

```go
(c *Catalog) GetTable(name string) (*TableDef, error)
(c *Catalog) ListTables() []*TableDef
```
Pure in-memory map lookups; no disk access.

```go
(c *Catalog) CreateIndex(def *IndexDef) error
(c *Catalog) DropIndex(name string) error
(c *Catalog) GetIndex(name string) (*IndexDef, error)
(c *Catalog) IndexesForTable(tableName string) ([]*IndexDef, error)
```
Same pattern as the table methods. `DropIndex` removes the entry from both
`indexes` and `tableIndexes[def.TableName]`. `IndexesForTable` uses the
`tableIndexes` reverse map for O(1) table lookup.

---

## Phase 3 Status — complete

| Component | Status |
|-----------|--------|
| Schema types (`TableDef`, `IndexDef`, `ColumnDef`, `ForeignKeyDef`, `Value`) | Done |
| Binary encoding: `encodeTableDef` / `decodeTableDef` | Done |
| Binary encoding: `encodeIndexDef` / `decodeIndexDef` | Done |
| `Open` — scan catalog tree, populate in-memory maps | Done |
| `CreateTable` / `DropTable` / `GetTable` / `ListTables` | Done |
| `CreateIndex` / `DropIndex` / `GetIndex` / `IndexesForTable` | Done |
| `tableIndexes` reverse map for O(k) `DropTable` | Done |
| `AllocateNewRoot` on `BTree` for clean page allocation | Done |
| Tests (16 cases — CRUD, duplicate errors, cascade drop, re-open) | Done |

### Not in Phase 3 (later work)
- The catalog tree's own root page ID must be written to the file header (offset 15)
  and read back on `Open` — this wires up the pager header to the catalog.
- Foreign key enforcement — the catalog stores FK metadata but the executor (Phase 4)
  is responsible for checking referential integrity at write time.
- Schema migrations (ALTER TABLE) — not planned for the current scope.
