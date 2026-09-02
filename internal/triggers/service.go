// Package triggers implements the platform's trigger runtime: it wires a
// project's configured triggers (SCHEMA.md `triggers`) to the backing database
// adapter, subscribes to change events, and dispatches each event to its
// action — a webhook HTTP call or a sandboxed user function.
package triggers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/openbase/openbase/internal/adapter"
	"github.com/openbase/openbase/internal/function"
	"github.com/openbase/openbase/internal/metadata"
)

// Action runs the action associated with a trigger: a webhook or a function.
type Action interface {
	// Run executes the action for one event. It returns the outcome description.
	Run(ctx context.Context, trigger metadata.Trigger, event adapter.TriggerEvent, record map[string]any) error
}

// Dispatcher looks up and dispatches an event to matching triggers. It is
// separate from the Service so the pure dispatch logic is unit-testable with a
// fake store and action.
type Dispatcher struct {
	Store  metadata.Store
	Action Action
	Log    *slog.Logger
}

// DispatchGroup routes each trigger to the Action for its action type.
type DispatchGroup struct {
	Actions []Action
	Log     *slog.Logger
}

// Run picks the first Action matching the trigger's action type.
func (g *DispatchGroup) Run(ctx context.Context, t metadata.Trigger, event adapter.TriggerEvent, record map[string]any) error {
	for _, a := range g.Actions {
		if actionMatch(a, t) {
			return a.Run(ctx, t, event, record)
		}
	}
	return fmt.Errorf("no action configured for type %q", t.ActionType)
}

// actionMatch reports whether an Action handles a trigger (webhook actions only
// run webhook triggers, function actions only function triggers).
func actionMatch(a Action, t metadata.Trigger) bool {
	switch a.(type) {
	case *EndpointAction:
		return t.ActionType == metadata.ActionWebhook
	case *FunctionAction:
		return t.ActionType == metadata.ActionFunction
	default:
		return false
	}
}

// Dispatch runs every enabled trigger on (collection, event) for the project,
// passing the record through.
func (d *Dispatcher) Dispatch(ctx context.Context, projectID, collection string, event adapter.TriggerEvent, record map[string]any) {
	if d.Store == nil || d.Action == nil {
		return
	}
	triggers, err := d.Store.ListTriggers(ctx, projectID)
	if err != nil {
		if d.Log != nil {
			d.Log.Error("trigger: list triggers", "project", projectID, "err", err)
		}
		return
	}
	for _, t := range triggers {
		if !t.Enabled || t.Collection != collection || metadata.TriggerEvent(event) != t.Event {
			continue
		}
		if err := d.Action.Run(ctx, t, event, record); err != nil {
			if d.Log != nil {
				d.Log.Error("trigger: action failed", "project", projectID, "trigger", t.ID, "err", err)
			}
			continue
		}
	}
}

// EndpointAction implements Action for webhook triggers: POST the event to the
// configured URL as JSON.
type EndpointAction struct {
	Client *http.Client
	Log    *slog.Logger
}

func (e *EndpointAction) Run(ctx context.Context, t metadata.Trigger, event adapter.TriggerEvent, record map[string]any) error {
	if t.ActionType != metadata.ActionWebhook {
		return fmt.Errorf("endpoint action requires a webhook trigger")
	}
	body := map[string]any{
		"trigger_id": t.ID,
		"project_id": t.ProjectID,
		"collection": t.Collection,
		"event":      string(event),
		"data":       record,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	client := e.Client
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.ActionTarget, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}
	return nil
}

// FunctionAction implements Action for function triggers using the function
// runner.
type FunctionAction struct {
	Store  metadata.Store
	Runner function.Runner
	Log    *slog.Logger
}

func (f *FunctionAction) Run(ctx context.Context, t metadata.Trigger, event adapter.TriggerEvent, record map[string]any) error {
	if t.ActionType != metadata.ActionFunction {
		return fmt.Errorf("function action requires a function trigger")
	}
	fn, err := f.Store.GetFunction(ctx, t.ProjectID, t.ActionTarget)
	if err != nil {
		return fmt.Errorf("function %s: %w", t.ActionTarget, err)
	}
	payload := map[string]any{
		"trigger_id": t.ID,
		"project_id": t.ProjectID,
		"collection": t.Collection,
		"event":      string(event),
		"data":       record,
	}
	if f.Runner == nil {
		return fmt.Errorf("no function runner configured")
	}
	_, err = f.Runner.Run(ctx, fn.Source, string(fn.Runtime), payload)
	return err
}

// ---- Adapter subscription wiring ----

// DB is the subset of the adapter the service needs to wire subscriptions.
type DB interface {
	RegisterTrigger(ctx context.Context, t adapter.TriggerDefinition) error
	SubscribeToChanges(ctx context.Context, collection string, handler adapter.ChangeHandler) (adapter.Subscription, error)
	Disconnect(ctx context.Context) error
}

// Connector connects a project's database adapter, given its decrypted secret.
type Connector interface {
	Connect(ctx context.Context, conn metadata.Connection, secret metadata.ConnectionSecret) (DB, error)
}

// Service wires a project's triggers onto its adapter and forwards events to a
// Dispatcher. One goroutine runs per collection, fed by the adapter's
// subscription.
type Service struct {
	Store    metadata.Store
	Connect  Connector
	Dispatch *Dispatcher
	Log      *slog.Logger

	mu    sync.Mutex
	conns map[string]*projectConn
}

type projectConn struct {
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewService builds a trigger runtime with the given injectable dependencies.
func NewService(store metadata.Store, conn Connector, dispatch *Dispatcher, log *slog.Logger) *Service {
	return &Service{
		Store:    store,
		Connect:  conn,
		Dispatch: dispatch,
		Log:      log,
		conns:    map[string]*projectConn{},
	}
}

// RegisterProject connects the project adapter, registers its native DB
// triggers, and starts change subscriptions for every enabled trigger.
// It is idempotent: calling again reloads (re-registers) with the latest config.
func (s *Service) RegisterProject(ctx context.Context, conn metadata.Connection, secret metadata.ConnectionSecret) error {
	s.StopProject(conn.ProjectID)

	db, err := s.Connect.Connect(ctx, conn, secret)
	if err != nil {
		return fmt.Errorf("trigger: connect: %w", err)
	}

	triggers, err := s.Store.ListTriggers(ctx, conn.ProjectID)
	if err != nil {
		_ = db.Disconnect(ctx)
		return err
	}

	subCtx, cancel := context.WithCancel(context.Background())
	pc := &projectConn{cancel: cancel}

	for _, t := range triggers {
		if !t.Enabled {
			continue
		}
		def := adapter.TriggerDefinition{
			ID:           t.ID,
			ProjectID:    t.ProjectID,
			Name:         t.Name,
			Collection:   t.Collection,
			Event:        adapter.TriggerEvent(t.Event),
			ActionType:   string(t.ActionType),
			ActionTarget: t.ActionTarget,
		}
		if err := db.RegisterTrigger(ctx, def); err != nil {
			if s.Log != nil {
				s.Log.Warn("trigger: register failed (continuing)", "trigger", t.ID, "err", err)
			}
			continue
		}
	}

	s.mu.Lock()
	s.conns[conn.ProjectID] = pc
	s.mu.Unlock()

	// One listening goroutine per unique collection among enabled triggers. They
	// all share the one cancel; a WaitGroup tracks when they've all returned.
	seen := map[string]bool{}
	for _, t := range triggers {
		if !t.Enabled || seen[t.Collection] {
			continue
		}
		seen[t.Collection] = true
		pc.wg.Add(1)
		collection := t.Collection
		projectID := conn.ProjectID
		go func() {
			defer pc.wg.Done()
			defer func() { _ = db.Disconnect(subCtx) }()
			sub, err := db.SubscribeToChanges(subCtx, collection,
				func(c string, ev adapter.TriggerEvent, rec map[string]any) {
					s.Dispatch.Dispatch(context.Background(), projectID, c, ev, rec)
				})
			if err != nil {
				if s.Log != nil {
					s.Log.Error("trigger: subscribe", "project", projectID, "collection", collection, "err", err)
				}
				return
			}
			<-subCtx.Done()
			_ = sub.Close()
		}()
	}

	return nil
}

// StopProject tears down subscriptions for a project (if any).
func (s *Service) StopProject(projectID string) {
	s.mu.Lock()
	pc := s.conns[projectID]
	delete(s.conns, projectID)
	s.mu.Unlock()
	if pc != nil {
		pc.cancel()
		waitTimeout(&pc.wg, 2*time.Second)
	}
}

// Stop tears down all projects.
func (s *Service) Stop() {
	s.mu.Lock()
	pcs := make([]*projectConn, 0, len(s.conns))
	for _, pc := range s.conns {
		pcs = append(pcs, pc)
	}
	s.conns = map[string]*projectConn{}
	s.mu.Unlock()
	for _, pc := range pcs {
		pc.cancel()
	}
	for _, pc := range pcs {
		waitTimeout(&pc.wg, 2*time.Second)
	}
}

// waitTimeout waits for a WaitGroup with a deadline.
func waitTimeout(wg *sync.WaitGroup, d time.Duration) {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(d):
	}
}
