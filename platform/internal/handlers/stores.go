package handlers

import (
	"database/sql"

	"github.com/weiboz0/bridge/platform/internal/store"
)

// Stores holds all store instances for dependency injection.
type Stores struct {
	Orgs  *store.OrgStore
	Users *store.UserStore
}

// NewStores creates all stores from a database connection.
func NewStores(db *sql.DB) *Stores {
	return &Stores{
		Orgs:  store.NewOrgStore(db),
		Users: store.NewUserStore(db),
	}
}
