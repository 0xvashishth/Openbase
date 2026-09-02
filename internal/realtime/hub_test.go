package realtime

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/openbase/openbase/internal/adapter"
)

type fakeSub struct {
	closed chan struct{}
	once   sync.Once
}

func (f *fakeSub) Close() error {
	f.once.Do(func() { close(f.closed) })
	return nil
}

// fakeConnector captures the change handler per collection so tests can fire
// events as if the database notified (retains handlers across subscribe calls).
type fakeConnector struct {
	mu       sync.Mutex
	handlers map[string]adapter.ChangeHandler
	subs     map[string]*fakeSub
}

func (f *fakeConnector) ConnectForProject(ctx context.Context, projectID, collection string, handler adapter.ChangeHandler) (adapter.Subscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.handlers == nil {
		f.handlers = map[string]adapter.ChangeHandler{}
		f.subs = map[string]*fakeSub{}
	}
	f.handlers[collection] = handler
	sub := &fakeSub{closed: make(chan struct{})}
	f.subs[collection] = sub
	return sub, nil
}

// fire invokes the retained handler for a collection (database changed).
func (f *fakeConnector) fire(projectID, collection string) bool {
	f.mu.Lock()
	h := f.handlers[collection]
	f.mu.Unlock()
	if h == nil {
		return false
	}
	h(collection, adapter.TriggerInsert, map[string]any{"id": 1, "name": "live"})
	return true
}

// fakeSink lets tests observe when a client is closed.
type fakeSink struct {
	closed chan struct{}
}

func newFakeSink() *fakeSink { return &fakeSink{closed: make(chan struct{})} }
func (f *fakeSink) Close()   { close(f.closed) }

// collect drains exactly n messages from a client's queue.
func collect(c *Client, n int, t *testing.T) []map[string]any {
	var out []map[string]any
	for i := 0; i < n; i++ {
		select {
		case raw := <-c.Messages():
			var msg map[string]any
			if err := json.Unmarshal(raw, &msg); err != nil {
				t.Fatalf("bad message: %v", err)
			}
			out = append(out, msg)
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for message %d", i)
		}
	}
	return out
}

func TestSubscribeReceivesChange(t *testing.T) {
	conn := &fakeConnector{}
	h := NewHub(nil, conn, nil)
	client := h.Attach("proj", newFakeSink())
	defer client.Detach()

	client.Handle([]byte(`{"type":"subscribe","collection":"orders"}`))
	ack := collect(client, 1, t)[0]
	if ack["type"] != "subscribed" || ack["collection"] != "orders" {
		t.Fatalf("expected subscribed ack, got %v", ack)
	}

	if !conn.fire("proj", "orders") {
		t.Fatal("expected handler retained for orders")
	}
	change := collect(client, 1, t)[0]
	if change["type"] != "change" || change["collection"] != "orders" || change["event"] != "insert" {
		t.Fatalf("unexpected change message: %v", change)
	}
	data, _ := change["data"].(map[string]any)
	if data["name"] != "live" {
		t.Fatalf("expected live row data, got %v", data)
	}
}

func TestOnlySubscribedCollectionReceivesChange(t *testing.T) {
	conn := &fakeConnector{}
	h := NewHub(nil, conn, nil)
	client := h.Attach("proj", newFakeSink())
	defer client.Detach()

	// Subscribe to orders only.
	client.Handle([]byte(`{"type":"subscribe","collection":"orders"}`))
	collect(client, 1, t)

	// Change on items (not subscribed) must not be delivered.
	if conn.fire("proj", "items") {
		t.Fatal("expected fire only possible for retained (subscribed) handler")
	}
	select {
	case m := <-client.Messages():
		t.Fatalf("unexpected message for unsubscribed collection: %v", m)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestBadAndUnknownMessages(t *testing.T) {
	conn := &fakeConnector{}
	h := NewHub(nil, conn, nil)
	client := h.Attach("proj", newFakeSink())
	defer client.Detach()

	client.Handle([]byte(`not-json`))
	if collect(client, 1, t)[0]["type"] != "error" {
		t.Fatal("expected error for bad json")
	}
	client.Handle([]byte(`{"type":"subscribe","collection":""}`))
	if collect(client, 1, t)[0]["type"] != "error" {
		t.Fatal("expected error for empty collection")
	}
	client.Handle([]byte(`{"type":"ping"}`))
	if collect(client, 1, t)[0]["type"] != "error" {
		t.Fatal("expected error for unknown type")
	}
}

func TestUnsubscribeClosesSharedSubscription(t *testing.T) {
	conn := &fakeConnector{}
	h := NewHub(nil, conn, nil)
	client := h.Attach("proj", newFakeSink())
	defer client.Detach()

	client.Handle([]byte(`{"type":"subscribe","collection":"items"}`))
	collect(client, 1, t)
	conn.mu.Lock()
	sub := conn.subs["items"]
	conn.mu.Unlock()
	if sub == nil {
		t.Fatal("expected subscription created")
	}

	client.Handle([]byte(`{"type":"unsubscribe","collection":"items"}`))
	collect(client, 1, t)

	select {
	case <-sub.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("expected shared subscription to close after client leaves")
	}
}
