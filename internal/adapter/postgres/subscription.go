package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// pgSubscription owns a dedicated LISTEN connection until Close.
type pgSubscription struct {
	conn   *pgxpool.Conn
	cancel context.CancelFunc
}

func (s *pgSubscription) Close() error {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if s.conn != nil {
		s.conn.Release()
		s.conn = nil
	}
	return nil
}