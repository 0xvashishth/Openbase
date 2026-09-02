package realtime

import (
	"context"
	"net/http"

	"github.com/coder/websocket"
)

// wsSink lets the hub close the underlying socket on detach.
type wsSink struct {
	conn *websocket.Conn
}

func (w *wsSink) Close() { _ = w.conn.Close(websocket.StatusNormalClosure, "closed") }

// ServeWS runs the WebSocket exchange for one upgraded connection, scoped to
// the given project (already resolved from the API key). It blocks until the
// client disconnects; followers of the project's collections receive live
// change events.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request, projectID string) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		return
	}
	sink := &wsSink{conn: conn}
	client := h.Attach(projectID, sink)
	defer client.Detach()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go writer(ctx, conn, client)

	// Reader loop: each text/binary frame is a control message.
	for {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		if typ == websocket.MessageText || typ == websocket.MessageBinary {
			client.Handle(data)
		}
	}
}

// writer drains a client's outbound queue to the socket until the connection
// is closed or the context is done.
func writer(ctx context.Context, conn *websocket.Conn, client *Client) {
	for {
		select {
		case <-ctx.Done():
			_ = conn.Close(websocket.StatusNormalClosure, "bye")
			return
		case payload, ok := <-client.Messages():
			if !ok {
				return
			}
			_ = conn.Write(ctx, websocket.MessageText, payload)
		}
	}
}
