package engine

import (
	"context"
	"testing"

	"github.com/openbase/openbase/internal/adapter"
	"github.com/openbase/openbase/internal/testutil"
)

func TestTestConnectionPostgres(t *testing.T) {
	pg := testutil.StartPostgres(t)
	f := NewFactory()

	eng, err := f.TestConnection(context.Background(), pg.DSN)
	if err != nil {
		t.Fatalf("TestConnection: %v", err)
	}
	if eng != adapter.EnginePostgres {
		t.Fatalf("engine = %q, want postgres", eng)
	}
}

func TestTestConnectionBadString(t *testing.T) {
	f := NewFactory()
	if _, err := f.TestConnection(context.Background(), "garbage"); err == nil {
		t.Fatal("garbage connection string should fail detection")
	}
}

func TestConnectForEngineUnsupported(t *testing.T) {
	f := NewFactory()
	_, err := f.ConnectForEngine(context.Background(), adapter.EngineFerretDB, adapter.ConnectionConfig{})
	if err == nil {
		t.Fatal("unsupported engine should error")
	}
}

func TestConnectForPostgresWrongHostFails(t *testing.T) {
	f := NewFactory()
	_, err := f.ConnectForEngine(context.Background(), adapter.EnginePostgres, adapter.ConnectionConfig{
		ConnStr: "postgres://u:p@127.0.0.1:1/nope",
	})
	if err == nil {
		t.Fatal("unreachable host should fail")
	}
}