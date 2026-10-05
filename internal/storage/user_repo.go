package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/chivta/knubaschedulebot/internal/domain"
)

const (
	selectUserGroup = `SELECT group_id, group_name, faculty_id, course FROM users WHERE telegram_id = ?`
	upsertUserGroup = `INSERT INTO users (telegram_id, group_id, group_name, faculty_id, course, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (telegram_id) DO UPDATE SET
    group_id = excluded.group_id,
    group_name = excluded.group_name,
    faculty_id = excluded.faculty_id,
    course = excluded.course,
    updated_at = excluded.updated_at`
)

// UserRepo stores the group each Telegram user picked.
type UserRepo struct{ db *sql.DB }

// NewUserRepo returns a UserRepo backed by db.
func NewUserRepo(db *sql.DB) *UserRepo {
	return &UserRepo{db: db}
}

// Group returns the group the user picked, or domain.ErrNotFound when they have not picked one.
func (r *UserRepo) Group(ctx context.Context, userID int64) (domain.Group, error) {
	var g domain.Group
	err := r.db.QueryRowContext(ctx, selectUserGroup, userID).Scan(&g.ID, &g.Name, &g.FacultyID, &g.Course)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Group{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Group{}, fmt.Errorf("select user group: %w", err)
	}
	return g, nil
}

// SetGroup stores the user's group, replacing an earlier choice.
func (r *UserRepo) SetGroup(ctx context.Context, userID int64, group domain.Group) error {
	_, err := r.db.ExecContext(ctx, upsertUserGroup, userID, group.ID, group.Name, group.FacultyID, group.Course, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("upsert user group: %w", err)
	}
	return nil
}
