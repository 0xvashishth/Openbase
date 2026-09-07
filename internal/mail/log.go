package mail

import (
	"context"
	"log/slog"
)

// LogSender is the dev/honest-fallback sink: it records what WOULD be sent
// without touching the network. Unconfigured instances use it so signup and
// invites keep working (visibly degraded) instead of failing.
type LogSender struct {
	Log *slog.Logger
}

// NewLogSender builds the sink; a nil logger discards (tests).
func NewLogSender(log *slog.Logger) Sender { return &LogSender{Log: log} }

// Send logs the message. It only fails on a cancelled context — the sink
// itself is infallible by design.
func (l *LogSender) Send(ctx context.Context, msg Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if l.Log != nil {
		l.Log.Info("mail: logged (no provider configured)",
			"to", msg.To, "subject", msg.Subject)
	}
	return nil
}
