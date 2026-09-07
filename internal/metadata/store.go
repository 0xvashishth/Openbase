// Package metadata persists the platform's own bookkeeping state: users,
// organizations, memberships, projects, connections and API keys. This is
// independent of whatever database each project uses for application data.
// See SCHEMA.md.
package metadata

import (
	"context"
	"time"
)

// User is a platform account holder.
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	FullName     string    `json:"full_name,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// OrgRole enumerates membership roles.
type OrgRole string

const (
	RoleOwner  OrgRole = "owner"
	RoleAdmin  OrgRole = "admin"
	RoleMember OrgRole = "member"
)

// rank returns a numeric priority so higher roles can be compared with
// >=. owner > admin > member.
func (r OrgRole) rank() int {
	switch r {
	case RoleOwner:
		return 3
	case RoleAdmin:
		return 2
	case RoleMember:
		return 1
	default:
		return 0
	}
}

// AtLeast reports whether this role meets or exceeds the given minimum.
func (r OrgRole) AtLeast(min OrgRole) bool {
	return r.rank() >= min.rank()
}

// Organization groups projects under a single owner.
type Organization struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Membership links a user to an organization with a role.
type Membership struct {
	OrganizationID string    `json:"organization_id"`
	UserID         string    `json:"user_id"`
	Role           OrgRole   `json:"role"`
	JoinedAt       time.Time `json:"joined_at"`
}

// Project is a named, org-scoped unit with exactly one connection in v1.
type Project struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Name           string    `json:"name"`
	Slug           string    `json:"slug"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// ConnectionMode mirrors SCHEMA.md mode CHECK constraint.
type ConnectionMode string

const (
	ModeProvisioned ConnectionMode = "provisioned"
	ModeBYODB       ConnectionMode = "byodb"
)

// ConnectionStatus mirrors SCHEMA.md status CHECK constraint.
type ConnectionStatus string

const (
	StatusPending   ConnectionStatus = "pending"
	StatusConnected ConnectionStatus = "connected"
	StatusError     ConnectionStatus = "error"
)

// Connection holds one project's database connection config. Sensitive
// fields are stored encrypted; the store never returns plaintext.
type Connection struct {
	ID                    string           `json:"id"`
	ProjectID             string           `json:"project_id"`
	Mode                  ConnectionMode   `json:"mode"`
	Engine                string           `json:"engine"`
	ContainerID           *string          `json:"container_id,omitempty"`
	EncryptedConnString   []byte           `json:"-"`
	EncryptedUsername     []byte           `json:"-"`
	EncryptedPassword     []byte           `json:"-"`
	EncryptionKeyID       string           `json:"encryption_key_id,omitempty"`
	Status                ConnectionStatus `json:"status"`
	LastCheckedAt         *time.Time       `json:"last_checked_at,omitempty"`
	CreatedAt             time.Time        `json:"created_at"`
}

// APIKey is a per-project key for external app access. Only the hash is stored.
type APIKey struct {
	ID        string     `json:"id"`
	ProjectID string     `json:"project_id"`
	Name      string     `json:"name"`
	KeyHash   string     `json:"-"`
	Scopes    []string   `json:"scopes"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

// TriggerEvent mirrors SCHEMA.md triggers.event CHECK constraint.
type TriggerEvent string

const (
	TriggerInsert TriggerEvent = "insert"
	TriggerUpdate TriggerEvent = "update"
	TriggerDelete TriggerEvent = "delete"
)

// TriggerActionType mirrors SCHEMA.md triggers.action_type CHECK constraint.
type TriggerActionType string

const (
	ActionFunction TriggerActionType = "function"
	ActionWebhook  TriggerActionType = "webhook"
)

// Trigger is a per-project rule: when `event` happens on `collection`, invoke
// `action_type` with `action_target` (a function id or webhook URL).
type Trigger struct {
	ID           string            `json:"id"`
	ProjectID    string            `json:"project_id"`
	Name         string            `json:"name"`
	Collection   string            `json:"collection"`
	Event        TriggerEvent      `json:"event"`
	ActionType   TriggerActionType `json:"action_type"`
	ActionTarget string            `json:"action_target"`
	Enabled      bool              `json:"enabled"`
	CreatedAt    time.Time         `json:"created_at"`
}

// FunctionRuntime mirrors SCHEMA.md functions.runtime CHECK constraint.
type FunctionRuntime string

const (
	RuntimeNode   FunctionRuntime = "node"
	RuntimePython FunctionRuntime = "python"
)

// Function is a user-authored source code function stored with a source_ref.
type Function struct {
	ID        string          `json:"id"`
	ProjectID string          `json:"project_id"`
	Name      string          `json:"name"`
	Runtime   FunctionRuntime `json:"runtime"`
	Source    string          `json:"source"`
	CreatedAt time.Time       `json:"created_at"`
}

// ConnectionSecret holds decrypted credentials handed to an adapter at
// runtime; it is never persisted.
type ConnectionSecret struct {
	ConnString string
	Username   string
	Password   string
}

// OrgMember is a membership with user details for UI display.
type OrgMember struct {
	UserID   string    `json:"user_id"`
	Email    string    `json:"email"`
	FullName string    `json:"full_name,omitempty"`
	Role     OrgRole   `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

// Invite represents a pending organization invitation.
type Invite struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Email          string    `json:"email"`
	Role           OrgRole   `json:"role"`
	TokenHash      string    `json:"-"`
	InvitedBy      string    `json:"invited_by"`
	ExpiresAt      time.Time `json:"expires_at"`
	AcceptedAt     *time.Time `json:"accepted_at,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// Store is the metadata repository contract. Implementations back onto the
// platform's own Postgres database.
type Store interface {
	// Users.
	CreateUser(ctx context.Context, u *User) error
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserByID(ctx context.Context, id string) (*User, error)

	// Organizations + memberships.
	CreateOrganization(ctx context.Context, org *Organization, creatorID string, role OrgRole) error
	GetOrganization(ctx context.Context, id string) (*Organization, error)
	ListOrganizationsForUser(ctx context.Context, userID string) ([]Organization, error)
	UpdateOrganization(ctx context.Context, org *Organization) error
	DeleteOrganization(ctx context.Context, id string) error
	CountProjects(ctx context.Context, orgID string) (int, error)
	AddMember(ctx context.Context, m *Membership) error
	ListMembers(ctx context.Context, orgID string) ([]Membership, error)
	ListMembersWithUsers(ctx context.Context, orgID string) ([]OrgMember, error)
	GetMembership(ctx context.Context, orgID, userID string) (*Membership, error)
	UpdateMemberRole(ctx context.Context, orgID, userID string, role OrgRole) error
	RemoveMember(ctx context.Context, orgID, userID string) error
	CountOwners(ctx context.Context, orgID string) (int, error)
	TransferOwnership(ctx context.Context, orgID, fromUserID, toUserID string, demoteFrom bool) error

	// Invites.
	CreateInvite(ctx context.Context, i *Invite) error
	GetInviteByToken(ctx context.Context, tokenHash string) (*Invite, error)
	ListInvites(ctx context.Context, orgID string) ([]Invite, error)
	RevokeInvite(ctx context.Context, id string) error
	AcceptInvite(ctx context.Context, tokenHash string, userID string) (*Membership, error)
	CleanupExpiredInvites(ctx context.Context) (int, error)

	// Projects.
	CreateProject(ctx context.Context, p *Project) error
	GetProject(ctx context.Context, id string) (*Project, error)
	ListProjects(ctx context.Context, orgID string) ([]Project, error)
	UpdateProject(ctx context.Context, p *Project) error
	DeleteProject(ctx context.Context, id string) error

	// Connections.
	CreateConnection(ctx context.Context, c *Connection) error
	GetConnectionByProject(ctx context.Context, projectID string) (*Connection, error)
	UpdateConnection(ctx context.Context, c *Connection) error
	UpdateConnectionStatus(ctx context.Context, id string, status ConnectionStatus) error
	DeleteConnection(ctx context.Context, id string) error

	// API keys.
	CreateAPIKey(ctx context.Context, k *APIKey) error
	GetAPIKeyByHash(ctx context.Context, hash string) (*APIKey, error)
	ListAPIKeys(ctx context.Context, projectID string) ([]APIKey, error)
	RevokeAPIKey(ctx context.Context, id string) error

	// Triggers.
	CreateTrigger(ctx context.Context, t *Trigger) error
	GetTrigger(ctx context.Context, projectID, id string) (*Trigger, error)
	ListTriggers(ctx context.Context, projectID string) ([]Trigger, error)
	UpdateTrigger(ctx context.Context, t *Trigger) error
	DeleteTrigger(ctx context.Context, projectID, id string) error

	// Functions.
	CreateFunction(ctx context.Context, f *Function) error
	GetFunction(ctx context.Context, projectID, id string) (*Function, error)
	ListFunctions(ctx context.Context, projectID string) ([]Function, error)
	DeleteFunction(ctx context.Context, projectID, id string) error

	// Webhook deliveries (Phase 8.9 hardening).
	// GetOrCreateWebhookSecret returns the project's HMAC signing secret,
	// generating and persisting one on first use.
	GetOrCreateWebhookSecret(ctx context.Context, projectID string) (string, error)
	RecordWebhookDelivery(ctx context.Context, d *WebhookDelivery) error
	ListWebhookDeliveries(ctx context.Context, projectID string, triggerID string, limit int) ([]WebhookDelivery, error)

	// Mail settings + log (Phase 9.2 dashboard-managed mailer).
	GetMailSettings(ctx context.Context) (*MailSettings, error)
	UpdateMailSettings(ctx context.Context, s *MailSettings) error
	RecordMailLog(ctx context.Context, e *MailLog) error
	ListMailLog(ctx context.Context, limit int) ([]MailLog, error)

	// Password reset tokens (Phase 9.3 credential lifecycle).
	CreateResetPasswordToken(ctx context.Context, userID string, ttl time.Duration) (string, *ResetPasswordToken, error)
	GetResetPasswordToken(ctx context.Context, hash string) (*ResetPasswordToken, error)
	MarkResetPasswordTokenUsed(ctx context.Context, id string) error
	SetUserPassword(ctx context.Context, userID, passwordHash string) error

	// Audit.
	AppendAuditEvent(ctx context.Context, e *AuditEvent) error
}

// WebhookDelivery records one webhook trigger delivery attempt for the
// Triggers UI log.
type WebhookDelivery struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id"`
	TriggerID  string    `json:"trigger_id,omitempty"`
	TargetURL  string    `json:"target_url"`
	Collection string    `json:"collection"`
	Event      string    `json:"event"`
	Attempts   int       `json:"attempts"`
	StatusCode *int      `json:"status_code"`
	OK         bool      `json:"ok"`
	Error      string    `json:"error,omitempty"`
	DurationMs int64     `json:"duration_ms"`
	CreatedAt  time.Time `json:"created_at"`
}

// MailSettings is the operator's mail provider (Phase 9.2). The SMTP
// password is stored envelope-encrypted and is never exposed via the API.
type MailSettings struct {
	Provider          string `json:"provider"`
	SMTPHost          string `json:"smtp_host"`
	SMTPPort          int    `json:"smtp_port"`
	SMTPUsername      string `json:"smtp_username"`
	EncryptedPassword []byte `json:"-"`
	EncryptionKeyID   string `json:"-"`
	FromAddress       string `json:"from_address"`
	FromName          string `json:"from_name"`
}

// MailLog records one outbound send attempt for the dashboard delivery log.
type MailLog struct {
	ID        string    `json:"id"`
	ToAddress string    `json:"to_address"`
	Template  string    `json:"template"`
	Subject   string    `json:"subject"`
	OK        bool      `json:"ok"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// AuditEvent represents a single audit log entry.
type AuditEvent struct {
	ID              string         `json:"id"`
	ActorUserID     string         `json:"actor_user_id"`
	OrganizationID  string         `json:"organization_id,omitempty"`
	ProjectID       string         `json:"project_id,omitempty"`
	Action          string         `json:"action"`
	TargetType      string         `json:"target_type"`
	TargetID        string         `json:"target_id,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
	IP              string         `json:"ip,omitempty"`
	UserAgent       string         `json:"user_agent,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
}


