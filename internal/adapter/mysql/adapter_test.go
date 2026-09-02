package mysql

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/openbase/openbase/internal/adapter"
)

// newMock builds an Adapter backed by a sqlmock driver, so CRUD/schema methods
// can be verified against the SQL they produce without a live MySQL server.
func newMock(t *testing.T) (*Adapter, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &Adapter{db: db}, mock
}

func TestCapabilitiesHonest(t *testing.T) {
	a := &Adapter{}
	c := a.Capabilities()
	if !c.SupportsRelationalJoins {
		t.Error("MySQL supports joins")
	}
	if !c.SupportsForeignKeys {
		t.Error("MySQL supports foreign keys")
	}
	if c.SupportsTransactions != true {
		t.Error("MySQL supports transactions")
	}
	if !c.SupportsFullTextSearch {
		t.Error("MySQL supports full-text search")
	}
	// Honest: the platform's trigger/realtime delivery is not implemented yet.
	if c.SupportsNativeTriggers {
		t.Errorf("NativeTriggers should be false until queue-table delivery lands")
	}
	if c.SupportsRealtime != adapter.RealtimeNone {
		t.Errorf("Realtime = %q, want %q", c.SupportsRealtime, adapter.RealtimeNone)
	}
	if c.SupportsChangeStreams {
		t.Errorf("ChangeStreams should be false")
	}
}

func TestNotConnectedErrors(t *testing.T) {
	a := &Adapter{}
	ctx := context.Background()
	if _, err := a.ListCollections(ctx); err == nil {
		t.Fatal("expected not-connected error")
	}
	if _, err := a.GetSchema(ctx, "users"); err == nil {
		t.Fatal("expected not-connected error")
	}
	if _, err := a.Query(ctx, adapter.UniversalQuery{}); err == nil {
		t.Fatal("expected not-connected error")
	}
}

func TestListCollections(t *testing.T) {
	a, mock := newMock(t)
	rows := sqlmock.NewRows([]string{"table_name"}).
		AddRow("users").AddRow("posts")
	mock.ExpectQuery(`SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE\(\)`).
		WillReturnRows(rows)

	cols, err := a.ListCollections(context.Background())
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	if len(cols) != 2 || cols[0].Name != "users" || cols[1].Name != "posts" {
		t.Fatalf("got %+v", cols)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetSchema(t *testing.T) {
	a, mock := newMock(t)
	colRows := sqlmock.NewRows([]string{"column_name", "data_type", "is_nullable", "column_default"}).
		AddRow("id", "bigint", "NO", nil).
		AddRow("name", "varchar", "YES", nil)
	pkRows := sqlmock.NewRows([]string{"column_name"}).AddRow("id")
	mock.ExpectQuery(`SELECT column_name, data_type, is_nullable, column_default FROM information_schema.columns WHERE table_schema = DATABASE\(\) AND table_name = \? ORDER BY ordinal_position`).
		WithArgs("users").WillReturnRows(colRows)
	mock.ExpectQuery(`SELECT column_name FROM information_schema.key_column_usage WHERE table_schema = DATABASE\(\) AND table_name = \? AND constraint_name = 'PRIMARY'`).
		WithArgs("users").WillReturnRows(pkRows)

	info, err := a.GetSchema(context.Background(), "users")
	if err != nil {
		t.Fatalf("GetSchema: %v", err)
	}
	if len(info.Columns) != 2 {
		t.Fatalf("got %d columns", len(info.Columns))
	}
	if !info.Columns[0].IsPrimary {
		t.Errorf("id should be primary key")
	}
	if info.Columns[1].IsPrimary {
		t.Errorf("name should not be primary key")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetSchemaInvalidTable(t *testing.T) {
	a, _ := newMock(t)
	if _, err := a.GetSchema(context.Background(), "users; DROP TABLE x"); err == nil {
		t.Fatal("expected invalid table name error")
	}
}

func TestListRelationships(t *testing.T) {
	a, mock := newMock(t)
	rows := sqlmock.NewRows([]string{"table_name", "column_name", "referenced_table_name", "referenced_column_name"}).
		AddRow("posts", "user_id", "users", "id")
	mock.ExpectQuery(`SELECT.*FROM information_schema.key_column_usage kcu WHERE kcu.table_schema = DATABASE\(\) AND kcu.referenced_table_name IS NOT NULL`).
		WillReturnRows(rows)

	rels, err := a.ListRelationships(context.Background())
	if err != nil {
		t.Fatalf("ListRelationships: %v", err)
	}
	if len(rels) != 1 {
		t.Fatalf("got %d rels", len(rels))
	}
	r := rels[0]
	if r.FromCollection != "posts" || r.FromColumn != "user_id" || r.ToCollection != "users" || r.ToColumn != "id" {
		t.Fatalf("got %+v", r)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestQueryWithFilterOrderLimit(t *testing.T) {
	a, mock := newMock(t)
	cols := []string{"id", "name", "score"}
	rows := sqlmock.NewRows(cols).
		AddRow([]byte("1"), []byte("Ada"), []byte("99.5")).
		AddRow([]byte("2"), []byte("Lin"), []byte("88"))

	limit := 10
	mock.ExpectQuery("SELECT \\* FROM `users` WHERE `id` = \\? ORDER BY `score` DESC LIMIT 10").
		WithArgs(int64(7)).
		WillReturnRows(rows)

	q := adapter.UniversalQuery{
		Filter: adapter.Filter{
			Collection: "users",
			Conditions: []adapter.Condition{{Field: "id", Operator: adapter.OpEqual, Value: 7}},
			OrderBy:    []adapter.OrderBy{{Field: "score", Desc: true}},
			Limit:      &limit,
		},
	}
	res, err := a.Query(context.Background(), q)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(res.Columns) != 3 || len(res.Rows) != 2 {
		t.Fatalf("got %d cols %d rows", len(res.Columns), len(res.Rows))
	}
	if res.Rows[0]["score"] != 99.5 {
		t.Errorf("score = %v, want 99.5", res.Rows[0]["score"])
	}
	if res.Rows[1]["id"] != int64(2) {
		t.Errorf("id = %v, want 2", res.Rows[1]["id"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestQueryInvalidCollection(t *testing.T) {
	a, _ := newMock(t)
	_, err := a.Query(context.Background(), adapter.UniversalQuery{
		Filter: adapter.Filter{Collection: "bad; drop"},
	})
	if err == nil {
		t.Fatal("expected invalid collection error")
	}
}

func TestQueryInvalidField(t *testing.T) {
	a, _ := newMock(t)
	_, err := a.Query(context.Background(), adapter.UniversalQuery{
		Filter: adapter.Filter{
			Collection: "users",
			Conditions: []adapter.Condition{{Field: "x; drop", Operator: adapter.OpEqual, Value: 1}},
		},
	})
	if err == nil {
		t.Fatal("expected invalid field error")
	}
}

func TestInsertUsesAutoIncrementID(t *testing.T) {
	a, mock := newMock(t)
	mock.ExpectExec("INSERT INTO `users` \\(`name`, `age`\\) VALUES \\(\\?, \\?\\)").
		WithArgs("Ada", 36).
		WillReturnResult(sqlmock.NewResult(42, 1))

	res, err := a.Insert(context.Background(), "users", map[string]any{"name": "Ada", "age": 36})
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if res.ID != int64(42) {
		t.Errorf("ID = %v, want 42", res.ID)
	}
	if res.Generated["id"] != int64(42) {
		t.Errorf("generated id = %v", res.Generated["id"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestInsertBehavesDeterministicallyWithoutMapOrder(t *testing.T) {
	a, mock := newMock(t)
	mock.ExpectExec("INSERT INTO `users` \\(`name`\\) VALUES \\(\\?\\)").
		WithArgs("Ada").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if _, err := a.Insert(context.Background(), "users", map[string]any{"name": "Ada"}); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUpdate(t *testing.T) {
	a, mock := newMock(t)
	mock.ExpectExec("UPDATE `users` SET `active` = \\? WHERE `id` = \\?").
		WithArgs(true, int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 3))

	res, err := a.Update(context.Background(), adapter.Filter{
		Collection: "users",
		Conditions: []adapter.Condition{{Field: "id", Operator: adapter.OpEqual, Value: 7}},
	}, map[string]any{"active": true})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if res.MatchedCount != 3 {
		t.Errorf("MatchedCount = %d, want 3", res.MatchedCount)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDelete(t *testing.T) {
	a, mock := newMock(t)
	mock.ExpectExec("DELETE FROM `posts` WHERE `id` = \\?").
		WithArgs(int64(9)).
		WillReturnResult(sqlmock.NewResult(0, 2))

	res, err := a.Delete(context.Background(), adapter.Filter{
		Collection: "posts",
		Conditions: []adapter.Condition{{Field: "id", Operator: adapter.OpEqual, Value: 9}},
	})
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if res.DeletedCount != 2 {
		t.Errorf("DeletedCount = %d, want 2", res.DeletedCount)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEncodesContainsAsLike(t *testing.T) {
	a, mock := newMock(t)
	rows := sqlmock.NewRows([]string{"id"}).AddRow([]byte("1"))
	mock.ExpectQuery("WHERE `name` LIKE \\?").
		WithArgs("%Ada%").
		WillReturnRows(rows)
	if _, err := a.Query(context.Background(), adapter.UniversalQuery{
		Filter: adapter.Filter{
			Collection: "users",
			Conditions: []adapter.Condition{{Field: "name", Operator: adapter.OpContains, Value: "%Ada%"}},
		},
	}); err != nil {
		t.Fatalf("Query: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTriggersAndRealtimeUnsupported(t *testing.T) {
	a, _ := newMock(t)
	ctx := context.Background()

	if err := a.RegisterTrigger(ctx, adapter.TriggerDefinition{}); !errors.Is(err, adapter.ErrUnsupported) {
		t.Errorf("RegisterTrigger err = %v, want ErrUnsupported", err)
	}
	if err := a.RemoveTrigger(ctx, "x"); !errors.Is(err, adapter.ErrUnsupported) {
		t.Errorf("RemoveTrigger err = %v, want ErrUnsupported", err)
	}
	if err := a.RegisterRealtimeBroadcast(ctx, "users"); !errors.Is(err, adapter.ErrUnsupported) {
		t.Errorf("RegisterRealtimeBroadcast err = %v, want ErrUnsupported", err)
	}
	if _, err := a.SubscribeToChanges(ctx, "users", nil); !errors.Is(err, adapter.ErrUnsupported) {
		t.Errorf("SubscribeToChanges err = %v, want ErrUnsupported", err)
	}
}

func TestBytesToValue(t *testing.T) {
	cases := []struct {
		in   []byte
		want any
	}{
		{nil, nil},
		{[]byte("42"), int64(42)},
		{[]byte("-7"), int64(-7)},
		{[]byte("3.14"), 3.14},
		{[]byte("hello"), "hello"},
		{[]byte(""), ""},
	}
	for _, c := range cases {
		if got := bytesToValue(c.in); got != c.want {
			t.Errorf("bytesToValue(%q) = %v (%T), want %v (%T)", c.in, got, got, c.want, c.want)
		}
	}
}
