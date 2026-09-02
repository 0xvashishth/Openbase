// Package realtime implements Phase 5's WebSocket gateway. Clients subscribe to
// data changes on a project's collections and receive live change events. The
// gateway is API-key authenticated and scopes subscriptions to the project the
// key belongs to.
//
// The transport is intentionally behind a small interface so the hub core
// (subscription routing, change dispatch) is unit-testable without a live
// WebSocket peer; the net/http WebSocket handler lives in ws.go.
package realtime

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/openbase/openbase/internal/adapter"
)

// Message type strings on the WebSocket (and test) wire.
const (
	MsgSubscribe   = "subscribe"
	MsgUnsubscribe = "unsubscribe"
	MsgSubscribed  = "subscribed"
	MsgChange      = "change"
	MsgError       = "error"
)

// incoming is a client-to-server control message.
type incoming struct {
	Type       string `json:"type"`
	Collection string `json:"collection"`
}

// outbound is a server-to-client message.
type outbound struct {
	Type       string         `json:"type"`
	Collection string         `json:"collection,omitempty"`
	Event      string         `json:"event,omitempty"`
	Data       map[string]any `json:"data,omitempty"`
	Error      string         `json:"error,omitempty"`
}

// Sink is a connected client transport. The hub only needs a way to close it;
// outbound messages are delivered by the transport draining the client's
// Messages() channel.
type Sink interface {
	// Close tears the client connection down.
	Close()
}

// Connector connects a project's database adapter for change subscriptions.
type Connector interface {
	// ConnectForProject returns a connected adapter for change subscriptions.
	ConnectForProject(ctx context.Context, projectID string, collection string, handler adapter.ChangeHandler) (adapter.Subscription, error)
}

// Store is the subset of the metadata store the hub needs.
type Store interface {
	ProjectExists(ctx context.Context, projectID string) (bool, error)
}

// Hub routes change events to subscribed clients, keyed by project (resolved
// from the API key at upgrade time).
type Hub struct {
	store   Store
	connect Connector
	log     *slog.Logger

	mu    sync.Mutex
	projs map[string]*projectHub
}

// NewHub builds a realtime hub. store may be nil if project validation is not
// required; connect may be nil only in tests that never subscribe.
func NewHub(store Store, connect Connector, log *slog.Logger) *Hub {
	if log == nil {
		log = slog.Default()
	}
	if store == nil {
		store = nilStore{}
	}
	return &Hub{store: store, connect: connect, log: log, projs: map[string]*projectHub{}}
}

// Client is one connected peer scoped to a project.
type Client struct {
	hub       *Hub
	projectID string
	conn      Sink
	subs      map[string]struct{}
	send      chan []byte
}

// projectHub tracks clients and active collection subscriptions for a project.
// The underlying DB subscription is shared across all clients of the project.
type projectHub struct {
	clients map[*Client]struct{}
	subs    map[string]adapter.Subscription
}

// Attach registers a client with the hub for its project.
func (h *Hub) Attach(projectID string, conn Sink) *Client {
	c := &Client{
		hub:       h,
		projectID: projectID,
		conn:      conn,
		subs:      map[string]struct{}{},
		send:      make(chan []byte, 256),
	}
	h.mu.Lock()
	ph := h.projs[projectID]
	if ph == nil {
		ph = &projectHub{clients: map[*Client]struct{}{}, subs: map[string]adapter.Subscription{}}
		h.projs[projectID] = ph
	}
	ph.clients[c] = struct{}{}
	h.mu.Unlock()
	return c
}

// Handle processes one incoming client message (decode error or control op).
func (c *Client) Handle(payload []byte) {
	var msg incoming
	if err := json.Unmarshal(payload, &msg); err != nil {
		c.write(MsgError, "", "", nil, "invalid message")
		return
	}
	switch msg.Type {
	case MsgSubscribe:
		c.subscribe(msg.Collection)
	case MsgUnsubscribe:
		c.unsubscribe(msg.Collection)
	default:
		c.write(MsgError, "", "", nil, "unknown message type")
	}
}

// subscribe registers the collection for this client, creating a shared DB
// subscription for the project on first use.
func (c *Client) subscribe(collection string) {
	if collection == "" {
		c.write(MsgError, "", "", nil, "collection required")
		return
	}
	if _, ok := c.subs[collection]; ok {
		c.write(MsgSubscribed, collection, "", nil, "")
		return
	}
	h := c.hub
	h.mu.Lock()
	ph := h.projs[c.projectID]
	if ph.subs[collection] == nil {
		if h.connect == nil {
			h.mu.Unlock()
			c.write(MsgError, collection, "", nil, "realtime unavailable")
			return
		}
		sub, err := h.connect.ConnectForProject(context.Background(), c.projectID, collection,
			func(col string, ev adapter.TriggerEvent, rec map[string]any) {
				h.dispatch(c.projectID, col, ev, rec)
			})
		if err != nil {
			h.mu.Unlock()
			h.log.Warn("realtime: subscribe", "project", c.projectID, "collection", collection, "err", err)
			c.write(MsgError, collection, "", nil, "subscribe failed: "+err.Error())
			return
		}
		ph.subs[collection] = sub
	}
	h.mu.Unlock()

	c.subs[collection] = struct{}{}
	c.write(MsgSubscribed, collection, "", nil, "")
}

// unsubscribe removes the collection from this client and cleans up the shared
// DB subscription when the last project client leaves.
func (c *Client) unsubscribe(collection string) {
	if _, ok := c.subs[collection]; !ok {
		return
	}
	delete(c.subs, collection)
	c.write(MsgSubscribed, collection, "", nil, "")

	h := c.hub
	h.mu.Lock()
	ph := h.projs[c.projectID]
	if ph == nil || len(ph.clients) > 1 {
		h.mu.Unlock()
		return
	}
	// Only client left for this collection — drop the shared subscription.
	if sub := ph.subs[collection]; sub != nil {
		_ = sub.Close()
		delete(ph.subs, collection)
	}
	h.mu.Unlock()
}

// Detach removes a client and cleans up subscriptions no longer needed.
func (c *Client) Detach() {
	h := c.hub
	h.mu.Lock()
	ph := h.projs[c.projectID]
	if ph != nil {
		delete(ph.clients, c)
		if len(ph.clients) == 0 {
			for _, sub := range ph.subs {
				_ = sub.Close()
			}
			delete(h.projs, c.projectID)
		}
	}
	h.mu.Unlock()
	if c.conn != nil {
		c.conn.Close()
	}
}

// dispatch broadcasts a change to this project's clients subscribed to col.
func (h *Hub) dispatch(projectID, collection string, ev adapter.TriggerEvent, rec map[string]any) {
	h.mu.Lock()
	ph := h.projs[projectID]
	if ph == nil {
		h.mu.Unlock()
		return
	}
	var targets []*Client
	for c := range ph.clients {
		if _, ok := c.subs[collection]; ok {
			targets = append(targets, c)
		}
	}
	h.mu.Unlock()
	if len(targets) == 0 {
		return
	}
	payload, _ := json.Marshal(outbound{
		Type:       MsgChange,
		Collection: collection,
		Event:      string(ev),
		Data:       rec,
	})
	for _, c := range targets {
		c.sendTo(payload)
	}
}

func (c *Client) write(t, collection, event string, data map[string]any, errMsg string) {
	msg := outbound{Type: t, Collection: collection, Event: event, Data: data, Error: errMsg}
	payload, _ := json.Marshal(msg)
	c.sendTo(payload)
}

// sendTo queues a message to the client's send channel (drained by the
// transport/Serve). Drops when the client is too slow rather than blocking.
func (c *Client) sendTo(payload []byte) {
	select {
	case c.send <- payload:
	default:
	}
}

// Messages exposes the client's outbound queue for the transport (or tests).
func (c *Client) Messages() <-chan []byte { return c.send }

// nilStore always accepts a project (used when no store is injected).
type nilStore struct{}

func (nilStore) ProjectExists(ctx context.Context, projectID string) (bool, error) { return true, nil }

// CloseAll detaches every client and closes all shared subscriptions. Used on
// server shutdown and in tests.
func (h *Hub) CloseAll() {
	h.mu.Lock()
	projs := make([]*Client, 0)
	for projectID := range h.projs {
		for c := range h.projs[projectID].clients {
			projs = append(projs, c)
		}
	}
	h.mu.Unlock()
	for _, c := range projs {
		c.Detach()
	}
}
