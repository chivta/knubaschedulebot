package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/chivta/knubaschedulebot/internal/domain"
)

const (
	selectAllowed = `SELECT EXISTS (SELECT 1 FROM allowed_users WHERE telegram_id = ?)
OR EXISTS (SELECT 1 FROM allowed_usernames WHERE username = ?)`
	insertAllowed = `INSERT INTO allowed_users (telegram_id, added_by, added_at) VALUES (?, ?, ?)
ON CONFLICT (telegram_id) DO NOTHING`
	deleteAllowed = `DELETE FROM allowed_users WHERE telegram_id = ?`
	listAllowed   = `SELECT telegram_id FROM allowed_users ORDER BY telegram_id ASC`

	insertAllowedUsername = `INSERT INTO allowed_usernames (username, added_by, added_at) VALUES (?, ?, ?)
ON CONFLICT (username) DO NOTHING`
	deleteAllowedUsername = `DELETE FROM allowed_usernames WHERE username = ?`
	listAllowedUsernames  = `SELECT username FROM allowed_usernames ORDER BY username ASC`
)

// AccessRepo stores the allow list of Telegram users. Admins are not stored
// here, they come from config.
type AccessRepo struct{ db *sql.DB }

// NewAccessRepo returns an AccessRepo backed by db.
func NewAccessRepo(db *sql.DB) *AccessRepo {
	return &AccessRepo{db: db}
}

// IsAllowed reports whether the user is on the allow list, by ID or by
// username. username is lowercase without "@", or "" when the user has none.
func (r *AccessRepo) IsAllowed(ctx context.Context, userID int64, username string) (bool, error) {
	var allowed bool
	err := r.db.QueryRowContext(ctx, selectAllowed, userID, username).Scan(&allowed)
	if err != nil {
		return false, fmt.Errorf("select allowed user: %w", err)
	}
	return allowed, nil
}

// Allow is idempotent: allowing a user twice is not an error.
func (r *AccessRepo) Allow(ctx context.Context, userID, addedBy int64) error {
	_, err := r.db.ExecContext(ctx, insertAllowed, userID, addedBy, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("insert allowed user: %w", err)
	}
	return nil
}

// Revoke returns domain.ErrNotFound when the user was not on the list.
func (r *AccessRepo) Revoke(ctx context.Context, userID int64) error {
	res, err := r.db.ExecContext(ctx, deleteAllowed, userID)
	if err != nil {
		return fmt.Errorf("delete allowed user: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("count deleted users: %w", err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// List returns allowed user IDs in ascending order.
func (r *AccessRepo) List(ctx context.Context) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, listAllowed)
	if err != nil {
		return nil, fmt.Errorf("list allowed users: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		err = rows.Scan(&id)
		if err != nil {
			return nil, fmt.Errorf("scan allowed user: %w", err)
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("iterate allowed users: %w", err)
	}
	return ids, nil
}

// AllowUsername is idempotent, like Allow.
func (r *AccessRepo) AllowUsername(ctx context.Context, username string, addedBy int64) error {
	_, err := r.db.ExecContext(ctx, insertAllowedUsername, username, addedBy, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("insert allowed username: %w", err)
	}
	return nil
}

// RevokeUsername returns domain.ErrNotFound when the username was not on the list.
func (r *AccessRepo) RevokeUsername(ctx context.Context, username string) error {
	res, err := r.db.ExecContext(ctx, deleteAllowedUsername, username)
	if err != nil {
		return fmt.Errorf("delete allowed username: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("count deleted usernames: %w", err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// ListUsernames returns allowed usernames in ascending order.
func (r *AccessRepo) ListUsernames(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, listAllowedUsernames)
	if err != nil {
		return nil, fmt.Errorf("list allowed usernames: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		err = rows.Scan(&name)
		if err != nil {
			return nil, fmt.Errorf("scan allowed username: %w", err)
		}
		names = append(names, name)
	}
	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("iterate allowed usernames: %w", err)
	}
	return names, nil
}
