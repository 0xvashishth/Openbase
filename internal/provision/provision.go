// Package provision defines the ProvisionerInterface (ARCHITECTURE.md §2.7)
// and a Docker Compose-backed implementation. The interface keeps provisioning
// swappable — Compose now, Kubernetes later — without touching the rest of the
// platform.
package provision

import (
	"context"
	"net"
	"strconv"
)

// Engine identifies what kind of instance to provision.
type Engine string

// Instance holds the runtime facts a caller needs to connect to a provisioned
// database and later clean it up.
type Instance struct {
	// ContainerID is the opaque handle for Destroy (e.g. a Docker container id).
	ContainerID string
	// Engine is what was provisioned.
	Engine Engine
	// ConnString bypasses TLS (local), with embedded credentials.
	ConnString string
	// Volume is the named Docker volume for data persistence (if applicable).
	Volume string
}

// ProvisionerInterface creates and destroys dedicated database instances.
type ProvisionerInterface interface {
	// Provision starts a dedicated instance and returns connection details.
	// The returned ConnString embeds generated credentials.
	Provision(ctx context.Context, engine Engine) (Instance, error)
	// Destroy removes a previously provisioned instance.
	Destroy(ctx context.Context, containerID string) error
}

// freePort reserves a host port so the provisioner can publish a container.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func itoa(n int) string { return strconv.Itoa(n) }