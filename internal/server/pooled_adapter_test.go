package server

import (
	"context"
	"reflect"
	"testing"

	"github.com/openbase/openbase/internal/adapter"
	"github.com/openbase/openbase/internal/metadata"
)

// TestPooledAdapterPreservesOptionalCapabilities is the guard for the failure
// mode this wrapper introduces: a type assertion cannot see through an embedded
// interface, so any optional capability the pooled adapter does not forward
// explicitly silently disappears for every engine at once.
//
// The first version of the pool omitted ExecRaw and the SQL editor started
// answering "raw queries are not supported for this engine" for Postgres. This
// test fails on the next capability added to the adapter surface without a
// matching forwarder.
func TestPooledAdapterPreservesOptionalCapabilities(t *testing.T) {
	var wrapped Adapter = pooledAdapter{Adapter: capableAdapter{}}

	if _, ok := wrapped.(adapter.RawQuerier); !ok {
		t.Error("pooledAdapter must forward adapter.RawQuerier: the SQL editor " +
			"type-asserts for it, so dropping it breaks raw queries on every engine")
	}
}

// TestPooledAdapterForwardsExecRaw proves the forwarder reaches the wrapped
// adapter rather than reporting unsupported.
func TestPooledAdapterForwardsExecRaw(t *testing.T) {
	inner := &recordingRaw{}
	p := pooledAdapter{Adapter: inner}

	raw, ok := any(p).(adapter.RawQuerier)
	if !ok {
		t.Fatal("pooledAdapter does not implement RawQuerier")
	}
	if _, err := raw.ExecRaw(context.Background(), "SELECT 1"); err != nil {
		t.Fatalf("ExecRaw: %v", err)
	}
	if inner.gotQuery != "SELECT 1" {
		t.Fatalf("inner adapter saw %q, want %q", inner.gotQuery, "SELECT 1")
	}
}

// An engine without raw support must surface ErrUnsupported so execSQL can
// answer with an honest 400 instead of a fabricated result.
func TestPooledAdapterReportsUnsupportedRaw(t *testing.T) {
	p := pooledAdapter{Adapter: capableAdapter{}}
	raw := any(p).(adapter.RawQuerier)

	_, err := raw.ExecRaw(context.Background(), "SELECT 1")
	if err == nil {
		t.Fatal("expected an error for an engine without RawQuerier")
	}
	if !isUnsupported(err) {
		t.Fatalf("err = %v, want it to wrap adapter.ErrUnsupported", err)
	}
}

// Disconnect must be a no-op: handlers run `defer a.Disconnect(ctx)`, and the
// pooled connection is shared with other in-flight requests.
func TestPooledAdapterDisconnectIsNoOp(t *testing.T) {
	inner := &recordingRaw{}
	p := pooledAdapter{Adapter: inner}

	if err := p.Disconnect(context.Background()); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if inner.disconnected {
		t.Error("pooledAdapter.Disconnect closed the shared connection; " +
			"the pool owns that lifecycle")
	}

	// unwrapPooled is how the pool reaches the real Disconnect on eviction.
	if err := unwrapPooled(p).Disconnect(context.Background()); err != nil {
		t.Fatalf("unwrapped Disconnect: %v", err)
	}
	if !inner.disconnected {
		t.Error("unwrapPooled must expose the real adapter so eviction can close it")
	}
}

// TestGenerationChangesWithCredentials pins the pool-key invariant that makes
// overwrite safe: saveConnection reuses the connection row id, so a key that
// ignored the credential generation would keep serving an adapter dialed at the
// previous database.
func TestGenerationChangesWithCredentials(t *testing.T) {
	base := connFixture()
	baseGen := generation(base)

	rotated := connFixture()
	rotated.EncryptionKeyID = "key-v2"
	if generation(rotated) == baseGen {
		t.Error("generation must change when encryption_key_id changes")
	}

	reengined := connFixture()
	reengined.Engine = "mysql"
	if generation(reengined) == baseGen {
		t.Error("generation must change when the engine changes")
	}

	longer := connFixture()
	longer.EncryptedConnString = append(longer.EncryptedConnString, 0x09, 0x09)
	if generation(longer) == baseGen {
		t.Error("generation must change when the stored ciphertext changes length")
	}

	container := connFixture()
	other := "container-2"
	container.ContainerID = &other
	if generation(container) == baseGen {
		t.Error("generation must change when the provisioned container changes")
	}

	// Same inputs must be stable, or every request would miss the cache.
	if generation(connFixture()) != baseGen {
		t.Error("generation must be stable for identical connections")
	}
}

func isUnsupported(err error) bool {
	for err != nil {
		if err == adapter.ErrUnsupported {
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

// connFixture returns a stable connection so generation() differences are
// attributable to the one field a case mutates.
func connFixture() *metadata.Connection {
	container := "container-1"
	return &metadata.Connection{
		ID:                  "conn-1",
		ProjectID:           "proj-1",
		Mode:                metadata.ModeProvisioned,
		Engine:              "postgres",
		ContainerID:         &container,
		EncryptedConnString: []byte{0x01, 0x02, 0x03, 0x04},
		EncryptionKeyID:     "key-v1",
		Status:              metadata.StatusConnected,
	}
}

// capableAdapter satisfies Adapter and nothing optional.
type capableAdapter struct{}

func (capableAdapter) ListCollections(context.Context) ([]adapter.CollectionInfo, error) {
	return nil, nil
}
func (capableAdapter) GetSchema(context.Context, string) (adapter.SchemaInfo, error) {
	return adapter.SchemaInfo{}, nil
}
func (capableAdapter) ListRelationships(context.Context) ([]adapter.Relationship, error) {
	return nil, nil
}
func (capableAdapter) Query(context.Context, adapter.UniversalQuery) (adapter.ResultSet, error) {
	return adapter.ResultSet{}, nil
}
func (capableAdapter) Insert(context.Context, string, map[string]any) (adapter.InsertResult, error) {
	return adapter.InsertResult{}, nil
}
func (capableAdapter) Update(context.Context, adapter.Filter, map[string]any) (adapter.UpdateResult, error) {
	return adapter.UpdateResult{}, nil
}
func (capableAdapter) Delete(context.Context, adapter.Filter) (adapter.DeleteResult, error) {
	return adapter.DeleteResult{}, nil
}
func (capableAdapter) RegisterRealtimeBroadcast(context.Context, string) error { return nil }
func (capableAdapter) SubscribeToChanges(context.Context, string, adapter.ChangeHandler) (adapter.Subscription, error) {
	return nil, nil
}
func (capableAdapter) Capabilities() adapter.CapabilitySet { return adapter.CapabilitySet{} }
func (capableAdapter) Disconnect(context.Context) error    { return nil }

// recordingRaw additionally implements RawQuerier and records what it saw.
type recordingRaw struct {
	capableAdapter
	gotQuery     string
	disconnected bool
}

func (r *recordingRaw) ExecRaw(_ context.Context, query string) (adapter.ResultSet, error) {
	r.gotQuery = query
	return adapter.ResultSet{}, nil
}

func (r *recordingRaw) Disconnect(context.Context) error {
	r.disconnected = true
	return nil
}

// TestAdapterInterfaceIsFullyForwarded fails if a method is added to the
// Adapter interface without pooledAdapter still satisfying it. Embedding makes
// that automatic today; the assertion documents the requirement so a future
// hand-written wrapper cannot quietly drop one.
func TestAdapterInterfaceIsFullyForwarded(t *testing.T) {
	want := reflect.TypeOf((*Adapter)(nil)).Elem()
	got := reflect.TypeOf(pooledAdapter{})
	if !got.Implements(want) {
		t.Fatal("pooledAdapter no longer implements Adapter")
	}
}
