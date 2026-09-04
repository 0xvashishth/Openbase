package qdrant

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/openbase/openbase/internal/adapter"
)

// fakePoint mirrors a point held by the in-memory fake Qdrant server.
type fakePoint struct {
	ID      string         `json:"id"`
	Vector  []float64      `json:"vector"`
	Payload map[string]any `json:"payload"`
}

// fakeQdrant is a tiny in-memory stand-in for the Qdrant REST API, covering
// exactly the endpoints the adapter uses.
type fakeQdrant struct {
	mu     sync.Mutex
	size   int
	points map[string][]fakePoint

	// Recorded request details so raw-execution tests can assert what the
	// adapter actually sent.
	lastScroll       map[string]any
	lastSearchVector []float64
	lastDelete       map[string]any
	created          []string
	dropped          []string
}

func newFakeQdrant(size int) *fakeQdrant {
	return &fakeQdrant{size: size, points: map[string][]fakePoint{}}
}

func writeResult(w http.ResponseWriter, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"result": result, "status": "ok", "time": 0.001})
}

func (f *fakeQdrant) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/collections", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		names := make([]map[string]string, 0, len(f.points))
		for name := range f.points {
			names = append(names, map[string]string{"name": name})
		}
		writeResult(w, map[string]any{"collections": names})
	})
	mux.HandleFunc("/collections/", func(w http.ResponseWriter, r *http.Request) {
		rest := r.URL.Path[len("/collections/"):]
		// Name is everything up to the first "/" (no nested paths in Qdrant).
		name := rest
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			name = rest[:i]
		}
		resource := strings.TrimPrefix(rest, name)

		f.mu.Lock()
		defer f.mu.Unlock()
		switch resource {
		case "/points/scroll":
			var req struct {
				Filter map[string]any `json:"filter"`
				Limit  int            `json:"limit"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			f.lastScroll = req.Filter
			pts := f.points[name]
			if req.Limit > 0 && req.Limit < len(pts) {
				pts = pts[:req.Limit]
			}
			writeResult(w, map[string]any{"points": pts})
		case "/points/search":
			var req struct {
				Vector []float64      `json:"vector"`
				Limit  int            `json:"limit"`
				Filter map[string]any `json:"filter"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			f.lastSearchVector = req.Vector
			pts := f.points[name]
			if req.Limit > 0 && req.Limit < len(pts) {
				pts = pts[:req.Limit]
			}
			// Qdrant search returns a bare array in "result".
			writeResult(w, pts)
		case "/points/payload":
			var req struct {
				Payload map[string]any `json:"payload"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			for i := range f.points[name] {
				for k, v := range req.Payload {
					f.points[name][i].Payload[k] = v
				}
			}
			writeResult(w, nil)
		case "/points/delete":
			var req struct {
				Filter map[string]any `json:"filter"`
				Points []any          `json:"points"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			f.lastDelete = map[string]any{"filter": req.Filter, "points": req.Points}
			writeResult(w, nil)
		case "/points":
			if r.Method != http.MethodPut {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var req struct {
				Points []fakePoint `json:"points"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			f.points[name] = append(f.points[name], req.Points...)
			writeResult(w, nil)
		case "":
			switch r.Method {
			case http.MethodPut:
				var cfg map[string]any
				_ = json.NewDecoder(r.Body).Decode(&cfg)
				f.created = append(f.created, name)
				if _, ok := f.points[name]; !ok {
					f.points[name] = []fakePoint{}
				}
				writeResult(w, true)
			case http.MethodDelete:
				delete(f.points, name)
				f.dropped = append(f.dropped, name)
				writeResult(w, true)
			default:
				writeResult(w, map[string]any{
					"config": map[string]any{
						"params": map[string]any{
							"vectors": map[string]any{"size": f.size, "distance": "Cosine"},
						},
					},
				})
			}
		default:
			http.NotFound(w, r)
		}
	})
	return mux
}

// startAdapter points a fresh Adapter at a fake Qdrant server.
func startAdapter(t *testing.T) (*Adapter, *fakeQdrant) {
	t.Helper()
	fake := newFakeQdrant(3)
	srv := httptest.NewServer(fake.Handler())
	t.Cleanup(srv.Close)
	a := New()
	if err := a.Connect(context.Background(), adapter.ConnectionConfig{ConnStr: srv.URL}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	return a, fake
}

func TestCapabilitiesHonest(t *testing.T) {
	c := New().Capabilities()
	if !c.SupportsVectorSearch {
		t.Error("Qdrant supports vector search")
	}
	if c.SupportsRelationalJoins || c.SupportsForeignKeys || c.SupportsNativeTriggers ||
		c.SupportsChangeStreams || c.SupportsTransactions || c.SupportsFullTextSearch {
		t.Error("vector engine should not claim relational/document/trigger features")
	}
	if c.SupportsRealtime != adapter.RealtimeNone {
		t.Errorf("Realtime = %q, want none", c.SupportsRealtime)
	}
}

func TestConnectRejectsNonHTTP(t *testing.T) {
	a := New()
	if err := a.Connect(context.Background(), adapter.ConnectionConfig{ConnStr: "valkey://host"}); err == nil {
		t.Fatal("expected error for non-http scheme")
	}
}

func TestListCollections(t *testing.T) {
	a, fake := startAdapter(t)
	fake.mu.Lock()
	fake.points["books"] = []fakePoint{}
	fake.points["movies"] = []fakePoint{}
	fake.mu.Unlock()

	cols, err := a.ListCollections(context.Background())
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	if len(cols) != 2 {
		t.Fatalf("got %d collections", len(cols))
	}
	names := []string{cols[0].Name, cols[1].Name}
	sort.Strings(names)
	if names[0] != "books" || names[1] != "movies" {
		t.Fatalf("got %+v", names)
	}
}

func TestGetSchema(t *testing.T) {
	a, fake := startAdapter(t)
	fake.mu.Lock()
	fake.points["things"] = []fakePoint{
		{ID: "1", Vector: []float64{1, 2, 3}, Payload: map[string]any{"name": "a", "qty": 5}},
		{ID: "2", Vector: []float64{4, 5, 6}, Payload: map[string]any{"name": "b", "tag": "x"}},
	}
	fake.mu.Unlock()

	info, err := a.GetSchema(context.Background(), "things")
	if err != nil {
		t.Fatalf("GetSchema: %v", err)
	}
	if info.Collection != "things" {
		t.Errorf("collection = %q", info.Collection)
	}
	var hasVector, hasName, hasQty, hasTag bool
	for _, c := range info.Columns {
		switch c.Name {
		case "vector":
			hasVector = true
		case "name":
			hasName = true
		case "qty":
			hasQty = true
		case "tag":
			hasTag = true
		}
	}
	if !hasVector || !hasName || !hasQty || !hasTag {
		t.Errorf("columns = %+v, want vector/name/qty/tag", info.Columns)
	}
}

func TestQueryWithFilter(t *testing.T) {
	a, fake := startAdapter(t)
	fake.mu.Lock()
	fake.points["users"] = []fakePoint{
		{ID: "1", Vector: []float64{1, 0, 0}, Payload: map[string]any{"name": "Ada", "age": 36}},
		{ID: "2", Vector: []float64{0, 1, 0}, Payload: map[string]any{"name": "Lin", "age": 30}},
	}
	fake.mu.Unlock()

	res, err := a.Query(context.Background(), adapter.UniversalQuery{
		Filter: adapter.Filter{
			Collection: "users",
			Conditions: []adapter.Condition{{Field: "name", Operator: adapter.OpEqual, Value: "Ada"}},
		},
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(res.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(res.Rows))
	}
	if res.Rows[0]["name"] != "Ada" {
		t.Errorf("row name = %v, want Ada", res.Rows[0]["name"])
	}
	if res.Rows[0]["vector"] == nil {
		t.Errorf("expected vector column")
	}
}

func TestQueryUnsupportedOperator(t *testing.T) {
	a, _ := startAdapter(t)
	_, err := a.Query(context.Background(), adapter.UniversalQuery{
		Filter: adapter.Filter{
			Collection: "users",
			Conditions: []adapter.Condition{{Field: "name", Operator: adapter.OpContains, Value: "x"}},
		},
	})
	if err == nil {
		t.Fatal("expected unsupported-operator error")
	}
}

func TestInsert(t *testing.T) {
	a, fake := startAdapter(t)
	res, err := a.Insert(context.Background(), "cats", map[string]any{
		"id":     "42",
		"name":   "whiskers",
		"age":    3,
		"vector": []float64{1, 2, 3},
	})
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if res.ID != "42" {
		t.Errorf("ID = %v, want 42", res.ID)
	}
	fake.mu.Lock()
	pts := append([]fakePoint(nil), fake.points["cats"]...)
	fake.mu.Unlock()
	if len(pts) != 1 {
		t.Fatalf("expected 1 stored point, got %d", len(pts))
	}
	if pts[0].Payload["name"] != "whiskers" {
		t.Errorf("payload name = %v", pts[0].Payload["name"])
	}
}

func TestInsertRequiresVector(t *testing.T) {
	a, _ := startAdapter(t)
	if _, err := a.Insert(context.Background(), "cats", map[string]any{"id": "1", "name": "x"}); err == nil {
		t.Fatal("expected error when no vector supplied")
	}
}

func TestUpdateAndDelete(t *testing.T) {
	a, fake := startAdapter(t)
	ctx := context.Background()
	fake.mu.Lock()
	fake.points["users"] = []fakePoint{
		{ID: "1", Vector: []float64{1, 0, 0}, Payload: map[string]any{"name": "Ada", "active": true}},
	}
	fake.mu.Unlock()

	filter := adapter.Filter{
		Collection: "users",
		Conditions: []adapter.Condition{{Field: "name", Operator: adapter.OpEqual, Value: "Ada"}},
	}
	upd, err := a.Update(ctx, filter, map[string]any{"active": false})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if upd.MatchedCount != 1 {
		t.Errorf("MatchedCount = %d, want 1", upd.MatchedCount)
	}
	del, err := a.Delete(ctx, filter)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if del.DeletedCount != 1 {
		t.Errorf("DeletedCount = %d, want 1", del.DeletedCount)
	}
}

func TestTriggersAndRealtimeUnsupported(t *testing.T) {
	a, _ := startAdapter(t)
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

func TestListRelationshipsEmpty(t *testing.T) {
	a, _ := startAdapter(t)
	rels, err := a.ListRelationships(context.Background())
	if err != nil || len(rels) != 0 {
		t.Fatalf("rels=%v err=%v, want empty", rels, err)
	}
}
