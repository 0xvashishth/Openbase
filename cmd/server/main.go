// Command server is the Openbase platform API binary: it serves the REST
// control plane (auth, orgs, projects, connections) and hosts the adapter
// engine entry points.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openbase/openbase/internal/auth"
	"github.com/openbase/openbase/internal/config"
	"github.com/openbase/openbase/internal/engine"
	"github.com/openbase/openbase/internal/metadata"
	"github.com/openbase/openbase/internal/provision"
	"github.com/openbase/openbase/internal/secrets"
	"github.com/openbase/openbase/internal/server"
	"github.com/openbase/openbase/migrations"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Default()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Platform's own metadata database.
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return err
	}

	// Apply metadata schema migrations (idempotent; see SCHEMA.md).
	if err := migrations.Apply(ctx, pool); err != nil {
		return err
	}

	store := metadata.NewPostgres(pool)
	tokens := auth.NewTokenManager(cfg.JWTSecret, cfg.JWTAudience, cfg.JWTIssuer, cfg.TokenTTL)

	// Envelope-encryption provider for connection secrets (SCHEMA.md §2).
	// If no key is configured, secrets-dependent endpoints are disabled but the
	// rest of the API still works.
	var secretsProv server.SecretsProvider
	if cfg.EncryptionKey != "" {
		p, err := secrets.New(cfg.EncryptionKey, "openbase-master-key-v1")
		if err != nil {
			return err
		}
		secretsProv = p
	} else {
		log.Warn("OPENBASE_ENCRYPTION_KEY not set; connection saving/secrets are disabled")
	}

	svc := &server.Services{
		Store:          store,
		Tokens:         tokens,
		Log:            log,
		AdapterFactory: engine.NewFactory(),
		Secrets:        secretsProv,
		AllowedOrigins: cfg.AllowedOrigins,
	}

	// Provisioning (ARCHITECTURE.md §2.7): a Compose-backed provisioner that
	// creates dedicated per-project Postgres containers. Off unless Docker is
	// explicitly enabled.
	if cfg.ProvisioningEnabled {
		svc.Provisioner = &provision.ServerProvisioner{Inner: &provision.Compose{}}
		log.Info("provisioning enabled (docker-based)")
	} else {
		log.Warn("OPENBASE_PROVISIONER_ENABLED not set; 'provisioned' connections are disabled")
	}

	handler := server.New(svc)

	srv := &http.Server{
		Addr:    cfg.Addr,
		Handler: handler,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("server listening", "addr", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}