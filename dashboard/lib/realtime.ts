// Client SDK for the Openbase realtime gateway (Phase 5).
//
// Usage:
//   const client = openbaseRealtime("wss://api.example.com/v1/realtime", "ob_xxx");
//   const unsub = client.subscribe("orders", (change) => {
//     console.log(change.event, change.data);
//   });
//   ...later...
//   unsub(); client.close();
//
// The gateway is authenticated with an API key; subscriptions are scoped to the
// project the key belongs to.

export interface RealtimeChange<T = Record<string, unknown>> {
  type: "change";
  collection: string;
  event: "insert" | "update" | "delete";
  data: T;
}

export interface RealtimeError {
  type: "error";
  collection?: string;
  error?: string;
}

export interface RealtimeSubscribed {
  type: "subscribed";
  collection: string;
}

export type RealtimeMessage<T = Record<string, unknown>> =
  | RealtimeChange<T>
  | RealtimeError
  | RealtimeSubscribed;

export interface LiveClient {
  /** Subscribe to changes on a collection; returns an unsubscribe function. */
  subscribe<T = Record<string, unknown>>(
    collection: string,
    onMessage: (msg: RealtimeChange<T>) => void,
    onError?: (msg: RealtimeError) => void
  ): () => void;
  /** Close the socket and all subscriptions. */
  close(): void;
}

export interface LiveClientOptions {
  /** Called with true on socket open, false on close/error. */
  onStatus?: (connected: boolean) => void;
}

const RECONNECT_MS = 3000;

export function openbaseRealtime(url: string, apiKey: string, opts?: LiveClientOptions): LiveClient {
  const onStatus = opts?.onStatus;
  // Browsers cannot set an Authorization header on new WebSocket(), so the
  // key travels as a query parameter (accepted by requireAPIKey). Keys are
  // ob_<base64url> — URL-safe, no encoding edge cases beyond the standard one.
  const authedURL = url + (url.includes("?") ? "&" : "?") + "apiKey=" + encodeURIComponent(apiKey);
  const subs = new Map<string, { ok: (msg: RealtimeChange<any>) => void; err?: (m: RealtimeError) => void }>();
  let socket: WebSocket | null = null;
  let closed = false;

  function setStatus(connected: boolean) {
    try {
      onStatus?.(connected);
    } catch {
      // A throwing status callback must never break the socket loop.
    }
  }

  function send(obj: unknown) {
    if (socket && socket.readyState === WebSocket.OPEN) {
      socket.send(JSON.stringify(obj));
    }
  }

  function ensureSocket() {
    if (socket && (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING)) {
      return;
    }
    socket = new WebSocket(authedURL);
    socket.onopen = () => {
      setStatus(true);
      for (const collection of subs.keys()) {
        send({ type: "subscribe", collection });
      }
    };
    socket.onmessage = (ev) => {
      let msg: RealtimeMessage;
      try {
        msg = JSON.parse(ev.data as string);
      } catch {
        return;
      }
      if (msg.type === "change") {
        const sub = subs.get(msg.collection);
        if (sub) sub.ok(msg as RealtimeChange);
      } else if (msg.type === "error") {
        const sub = msg.collection ? subs.get(msg.collection) : undefined;
        if (sub && sub.err) sub.err(msg as RealtimeError);
      }
    };
    socket.onclose = () => {
      setStatus(false);
      if (!closed && subs.size > 0) {
        // Reconnect and resubscribe.
        setTimeout(ensureSocket, RECONNECT_MS);
      }
    };
    socket.onerror = () => {
      setStatus(false);
      if (socket) socket.close();
    };
  }

  return {
    subscribe(collection, onMessage, onError) {
      ensureSocket();
      subs.set(collection, { ok: onMessage, err: onError });
      send({ type: "subscribe", collection });
      return () => {
        subs.delete(collection);
        send({ type: "unsubscribe", collection });
      };
    },
    close() {
      closed = true;
      subs.clear();
      if (socket) socket.close();
    },
  };
}
