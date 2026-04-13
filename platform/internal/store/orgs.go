package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Org represents a row in the organizations table.
type Org struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Slug         string     `json:"slug"`
	Type         string     `json:"type"`
	Status       string     `json:"status"`
	ContactEmail string     `json:"contactEmail"`
	ContactName  string     `json:"contactName"`
	Domain       *string    `json:"domain"`
	Settings     string     `json:"settings"`
	VerifiedAt   *time.Time `json:"verifiedAt"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

// OrgMembership represents a row in the org_memberships table.
type OrgMembership struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"orgId"`
	UserID    string    `json:"userId"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	InvitedBy *string   `json:"invitedBy"`
	CreatedAt time.Time `json:"createdAt"`
}

// OrgMemberWithUser combines membership with user data (for list endpoints).
type OrgMemberWithUser struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"orgId"`
	UserID    string    `json:"userId"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
}

// UserMembershipWithOrg combines membership with org data (for user's orgs list).
type UserMembershipWithOrg struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"orgId"`
	UserID    string    `json:"userId"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	OrgName   string    `json:"orgName"`
	OrgSlug   string    `json:"orgSlug"`
	OrgStatus string    `json:"orgStatus"`
}

// CreateOrgInput is the input for creating an organization.
type CreateOrgInput struct {
	Name         string  `json:"name"`
	Slug         string  `json:"slug"`
	Type         string  `json:"type"`
	ContactEmail string  `json:"contactEmail"`
	ContactName  string  `json:"contactName"`
	Domain       *string `json:"domain,omitempty"`
}

// OrgStore provides database operations for organizations and memberships.
type OrgStore struct {
	db *sql.DB
}

// NewOrgStore creates a new OrgStore.
func NewOrgStore(db *sql.DB) *OrgStore {
	return &OrgStore{db: db}
}

// CreateOrg inserts a new organization and returns it.
func (s *OrgStore) CreateOrg(ctx context.Context, input CreateOrgInput) (*Org, error) {
	id := uuid.New().String()
	now := time.Now()
	settings := "{}"

	var org Org
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO organizations (id, name, slug, type, status, contact_email, contact_name, domain, settings, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, 'pending', $5, $6, $7, $8, $9, $10)
		 RETURNING id, name, slug, type, status, contact_email, contact_name, domain, settings, verified_at, created_at, updated_at`,
		id, input.Name, input.Slug, input.Type, input.ContactEmail, input.ContactName, input.Domain, settings, now, now,
	).Scan(&org.ID, &org.Name, &org.Slug, &org.Type, &org.Status, &org.ContactEmail, &org.ContactName, &org.Domain, &org.Settings, &org.VerifiedAt, &org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &org, nil
}

// GetOrg retrieves an organization by ID.
func (s *OrgStore) GetOrg(ctx context.Context, orgID string) (*Org, error) {
	var org Org
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, slug, type, status, contact_email, contact_name, domain, settings, verified_at, created_at, updated_at
		 FROM organizations WHERE id = $1`,
		orgID,
	).Scan(&org.ID, &org.Name, &org.Slug, &org.Type, &org.Status, &org.ContactEmail, &org.ContactName, &org.Domain, &org.Settings, &org.VerifiedAt, &org.CreatedAt, &org.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &org, nil
}

// GetOrgBySlug retrieves an organization by slug.
func (s *OrgStore) GetOrgBySlug(ctx context.Context, slug string) (*Org, error) {
	var org Org
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, slug, type, status, contact_email, contact_name, domain, settings, verified_at, created_at, updated_at
		 FROM organizations WHERE slug = $1`,
		slug,
	).Scan(&org.ID, &org.Name, &org.Slug, &org.Type, &org.Status, &org.ContactEmail, &org.ContactName, &org.Domain, &org.Settings, &org.VerifiedAt, &org.CreatedAt, &org.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &org, nil
}

// ListOrgs lists all organizations, optionally filtered by status.
func (s *OrgStore) ListOrgs(ctx context.Context, status string) ([]Org, error) {
	var rows *sql.Rows
	var err error
	if status != "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, name, slug, type, status, contact_email, contact_name, domain, settings, verified_at, created_at, updated_at
			 FROM organizations WHERE status = $1 ORDER BY created_at DESC`,
			status,
		)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, name, slug, type, status, contact_email, contact_name, domain, settings, verified_at, created_at, updated_at
			 FROM organizations ORDER BY created_at DESC`,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orgs []Org
	for rows.Next() {
		var org Org
		if err := rows.Scan(&org.ID, &org.Name, &org.Slug, &org.Type, &org.Status, &org.ContactEmail, &org.ContactName, &org.Domain, &org.Settings, &org.VerifiedAt, &org.CreatedAt, &org.UpdatedAt); err != nil {
			return nil, err
		}
		orgs = append(orgs, org)
	}
	if orgs == nil {
		orgs = []Org{}
	}
	return orgs, rows.Err()
}

// UpdateOrgInput contains mutable fields for updating an organization.
type UpdateOrgInput struct {
	Name         *string `json:"name,omitempty"`
	ContactEmail *string `json:"contactEmail,omitempty"`
	ContactName  *string `json:"contactName,omitempty"`
	Domain       *string `json:"domain,omitempty"`
}

// UpdateOrg updates mutable fields of an organization.
func (s *OrgStore) UpdateOrg(ctx context.Context, orgID string, input UpdateOrgInput) (*Org, error) {
	setClauses := []string{}
	args := []any{}
	argIdx := 1

	if input.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *input.Name)
		argIdx++
	}
	if input.ContactEmail != nil {
		setClauses = append(setClauses, fmt.Sprintf("contact_email = $%d", argIdx))
		args = append(args, *input.ContactEmail)
		argIdx++
	}
	if input.ContactName != nil {
		setClauses = append(setClauses, fmt.Sprintf("contact_name = $%d", argIdx))
		args = append(args, *input.ContactName)
		argIdx++
	}
	if input.Domain != nil {
		setClauses = append(setClauses, fmt.Sprintf("domain = $%d", argIdx))
		args = append(args, *input.Domain)
		argIdx++
	}

	if len(setClauses) == 0 {
		return s.GetOrg(ctx, orgID)
	}

	setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", argIdx))
	args = append(args, time.Now())
	argIdx++

	args = append(args, orgID)

	query := fmt.Sprintf(
		`UPDATE organizations SET %s WHERE id = $%d
		 RETURNING id, name, slug, type, status, contact_email, contact_name, domain, settings, verified_at, created_at, updated_at`,
		strings.Join(setClauses, ", "), argIdx,
	)

	var org Org
	err := s.db.QueryRowContext(ctx, query, args...).Scan(
		&org.ID, &org.Name, &org.Slug, &org.Type, &org.Status, &org.ContactEmail, &org.ContactName, &org.Domain, &org.Settings, &org.VerifiedAt, &org.CreatedAt, &org.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &org, nil
}

// UpdateOrgStatus updates an org's status and optionally sets verifiedAt.
func (s *OrgStore) UpdateOrgStatus(ctx context.Context, orgID string, status string) (*Org, error) {
	existing, err := s.GetOrg(ctx, orgID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, nil
	}

	now := time.Now()
	var org Org

	if status == "active" && existing.Type == "school" && existing.VerifiedAt == nil {
		err = s.db.QueryRowContext(ctx,
			`UPDATE organizations SET status = $1, verified_at = $2, updated_at = $3 WHERE id = $4
			 RETURNING id, name, slug, type, status, contact_email, contact_name, domain, settings, verified_at, created_at, updated_at`,
			status, now, now, orgID,
		).Scan(&org.ID, &org.Name, &org.Slug, &org.Type, &org.Status, &org.ContactEmail, &org.ContactName, &org.Domain, &org.Settings, &org.VerifiedAt, &org.CreatedAt, &org.UpdatedAt)
	} else {
		err = s.db.QueryRowContext(ctx,
			`UPDATE organizations SET status = $1, updated_at = $2 WHERE id = $3
			 RETURNING id, name, slug, type, status, contact_email, contact_name, domain, settings, verified_at, created_at, updated_at`,
			status, now, orgID,
		).Scan(&org.ID, &org.Name, &org.Slug, &org.Type, &org.Status, &org.ContactEmail, &org.ContactName, &org.Domain, &org.Settings, &org.VerifiedAt, &org.CreatedAt, &org.UpdatedAt)
	}
	if err != nil {
		return nil, err
	}
	return &org, nil
}

// AddMemberInput is the input for adding an org membership.
type AddMemberInput struct {
	OrgID     string  `json:"orgId"`
	UserID    string  `json:"userId"`
	Role      string  `json:"role"`
	Status    string  `json:"status"`
	InvitedBy *string `json:"invitedBy,omitempty"`
}

// AddOrgMember adds a membership record.
func (s *OrgStore) AddOrgMember(ctx context.Context, input AddMemberInput) (*OrgMembership, error) {
	id := uuid.New().String()
	status := input.Status
	if status == "" {
		status = "active"
	}
	var m OrgMembership
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO org_memberships (id, org_id, user_id, role, status, invited_by, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT DO NOTHING
		 RETURNING id, org_id, user_id, role, status, invited_by, created_at`,
		id, input.OrgID, input.UserID, input.Role, status, input.InvitedBy, time.Now(),
	).Scan(&m.ID, &m.OrgID, &m.UserID, &m.Role, &m.Status, &m.InvitedBy, &m.CreatedAt)
	if err == sql.ErrNoRows {
		// ON CONFLICT DO NOTHING -- already exists
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// ListOrgMembers lists all members of an org with user details.
func (s *OrgStore) ListOrgMembers(ctx context.Context, orgID string) ([]OrgMemberWithUser, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT om.id, om.org_id, om.user_id, om.role, om.status, om.created_at, u.name, u.email
		 FROM org_memberships om
		 INNER JOIN users u ON om.user_id = u.id
		 WHERE om.org_id = $1
		 ORDER BY om.created_at`,
		orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []OrgMemberWithUser
	for rows.Next() {
		var m OrgMemberWithUser
		if err := rows.Scan(&m.ID, &m.OrgID, &m.UserID, &m.Role, &m.Status, &m.CreatedAt, &m.Name, &m.Email); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	if members == nil {
		members = []OrgMemberWithUser{}
	}
	return members, rows.Err()
}

// GetUserMemberships lists all org memberships for a user with org details.
func (s *OrgStore) GetUserMemberships(ctx context.Context, userID string) ([]UserMembershipWithOrg, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT om.id, om.org_id, om.user_id, om.role, om.status, om.created_at, o.name, o.slug, o.status
		 FROM org_memberships om
		 INNER JOIN organizations o ON om.org_id = o.id
		 WHERE om.user_id = $1
		 ORDER BY om.created_at`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memberships []UserMembershipWithOrg
	for rows.Next() {
		var m UserMembershipWithOrg
		if err := rows.Scan(&m.ID, &m.OrgID, &m.UserID, &m.Role, &m.Status, &m.CreatedAt, &m.OrgName, &m.OrgSlug, &m.OrgStatus); err != nil {
			return nil, err
		}
		memberships = append(memberships, m)
	}
	if memberships == nil {
		memberships = []UserMembershipWithOrg{}
	}
	return memberships, rows.Err()
}

// GetUserRolesInOrg returns all active memberships for a user in an org.
func (s *OrgStore) GetUserRolesInOrg(ctx context.Context, orgID, userID string) ([]OrgMembership, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, org_id, user_id, role, status, invited_by, created_at
		 FROM org_memberships
		 WHERE org_id = $1 AND user_id = $2 AND status = 'active'`,
		orgID, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memberships []OrgMembership
	for rows.Next() {
		var m OrgMembership
		if err := rows.Scan(&m.ID, &m.OrgID, &m.UserID, &m.Role, &m.Status, &m.InvitedBy, &m.CreatedAt); err != nil {
			return nil, err
		}
		memberships = append(memberships, m)
	}
	return memberships, rows.Err()
}

// GetOrgMembership retrieves a single membership by its ID.
func (s *OrgStore) GetOrgMembership(ctx context.Context, membershipID string) (*OrgMembership, error) {
	var m OrgMembership
	err := s.db.QueryRowContext(ctx,
		`SELECT id, org_id, user_id, role, status, invited_by, created_at
		 FROM org_memberships WHERE id = $1`,
		membershipID,
	).Scan(&m.ID, &m.OrgID, &m.UserID, &m.Role, &m.Status, &m.InvitedBy, &m.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// UpdateMemberStatus updates a membership's status.
func (s *OrgStore) UpdateMemberStatus(ctx context.Context, membershipID, status string) (*OrgMembership, error) {
	var m OrgMembership
	err := s.db.QueryRowContext(ctx,
		`UPDATE org_memberships SET status = $1 WHERE id = $2
		 RETURNING id, org_id, user_id, role, status, invited_by, created_at`,
		status, membershipID,
	).Scan(&m.ID, &m.OrgID, &m.UserID, &m.Role, &m.Status, &m.InvitedBy, &m.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// RemoveOrgMember deletes a membership and returns the deleted record.
func (s *OrgStore) RemoveOrgMember(ctx context.Context, membershipID string) (*OrgMembership, error) {
	var m OrgMembership
	err := s.db.QueryRowContext(ctx,
		`DELETE FROM org_memberships WHERE id = $1
		 RETURNING id, org_id, user_id, role, status, invited_by, created_at`,
		membershipID,
	).Scan(&m.ID, &m.OrgID, &m.UserID, &m.Role, &m.Status, &m.InvitedBy, &m.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}
