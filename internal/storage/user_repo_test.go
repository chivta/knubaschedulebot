package storage_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/chivta/knubaschedulebot/internal/domain"
	"github.com/chivta/knubaschedulebot/internal/storage"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestUserRepoGroupNotFound(t *testing.T) {
	repo := storage.NewUserRepo(openTestDB(t))

	_, err := repo.Group(context.Background(), 1)
	if err != domain.ErrNotFound {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestUserRepoSetThenGet(t *testing.T) {
	ctx := context.Background()
	repo := storage.NewUserRepo(openTestDB(t))
	want := domain.Group{ID: 10, Name: "KN-21", FacultyID: 3, Course: 2}

	err := repo.SetGroup(ctx, 1, want)
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := repo.Group(ctx, 1)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestUserRepoSetOverwrites(t *testing.T) {
	ctx := context.Background()
	repo := storage.NewUserRepo(openTestDB(t))
	want := domain.Group{ID: 11, Name: "KN-31", FacultyID: 3, Course: 3}

	err := repo.SetGroup(ctx, 1, domain.Group{ID: 10, Name: "KN-21", FacultyID: 3, Course: 2})
	if err != nil {
		t.Fatalf("first set: %v", err)
	}
	err = repo.SetGroup(ctx, 1, want)
	if err != nil {
		t.Fatalf("second set: %v", err)
	}
	got, err := repo.Group(ctx, 1)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
