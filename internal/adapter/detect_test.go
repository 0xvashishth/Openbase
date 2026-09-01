package adapter

import (
	"errors"
	"testing"
)

func TestDetectEngine(t *testing.T) {
	tests := []struct {
		name    string
		connStr string
		want    Engine
		wantErr bool
	}{
		{"postgres", "postgres://user:pass@localhost:5432/db", EnginePostgres, false},
		{"postgresql", "postgresql://user@host/db", EnginePostgres, false},
		{"mysql", "mysql://user:pass@localhost:3306/db", EngineMySQL, false},
		{"mongodb", "mongodb://user@localhost:27017/db", EngineFerretDB, false},
		{"mongodb+srv", "mongodb+srv://user@cluster.example.com/db", EngineFerretDB, false},
		{"redis", "redis://localhost:6379/0", EngineValkey, false},
		{"valkey", "valkey://localhost:6379/0", EngineValkey, false},
		{"rediss", "rediss://localhost:6379/0", EngineValkey, false},
		{"bolt arcadedb", "bolt://localhost:2480", EngineArcadeDB, false},
		{"qdrant default port", "http://localhost:6333", EngineQdrant, false},
		{"qdrant grpc port", "https://qdrant.internal:6334", EngineQdrant, false},
		{"empty", "", "", true},
		{"unknown scheme", "oracle://host/db", "", true},
		{"bare http no qdrant port", "https://example.com", "", true},
		{"garbage", "not a url", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DetectEngine(tt.connStr)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("DetectEngine(%q) expected error, got engine %q", tt.connStr, got)
				}
				if !errors.Is(err, ErrUnknownEngine) {
					t.Fatalf("DetectEngine(%q) expected ErrUnknownEngine, got %v", tt.connStr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("DetectEngine(%q) unexpected error: %v", tt.connStr, err)
			}
			if got != tt.want {
				t.Fatalf("DetectEngine(%q) = %q, want %q", tt.connStr, got, tt.want)
			}
		})
	}
}