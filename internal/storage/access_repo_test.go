package storage_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/chivta/knubaschedulebot/internal/domain"
	"github.com/chivta/knubaschedulebot/internal/storage"
)

func TestAccessRepoAllowAndIsAllowed(t *testing.T) {
	ctx := context.Background()
	repo := storage.NewAccessRepo(openTestDB(t))

	err := repo.Allow(ctx, 5, 1)
	if err != nil {
		t.Fatalf("allow: %v", err)
	}
	ok, err := repo.IsAllowed(ctx, 5, "")
	if err != nil || !ok {
		t.Fatalf("IsAllowed(5) = %v, %v; want true", ok, err)
	}
	ok, err = repo.IsAllowed(ctx, 6, "")
	if err != nil || ok {
		t.Fatalf("IsAllowed(6) = %v, %v; want false", ok, err)
	}
}

func TestAccessRepoAllowTwice(t *testing.T) {
	ctx := context.Background()
	repo := storage.NewAccessRepo(openTestDB(t))

	err := repo.Allow(ctx, 5, 1)
	if err != nil {
		t.Fatalf("first allow: %v", err)
	}
	err = repo.Allow(ctx, 5, 2)
	if err != nil {
		t.Fatalf("second allow: %v", err)
	}
	ids, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !slices.Equal(ids, []int64{5}) {
		t.Fatalf("got %v, want [5]", ids)
	}
}

func TestAccessRepoRevoke(t *testing.T) {
	ctx := context.Background()
	repo := storage.NewAccessRepo(openTestDB(t))

	err := repo.Allow(ctx, 5, 1)
	if err != nil {
		t.Fatalf("allow: %v", err)
	}
	err = repo.Revoke(ctx, 5)
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	ok, err := repo.IsAllowed(ctx, 5, "")
	if err != nil || ok {
		t.Fatalf("IsAllowed after revoke = %v, %v; want false", ok, err)
	}
}

func TestAccessRepoRevokeUnknown(t *testing.T) {
	repo := storage.NewAccessRepo(openTestDB(t))

	err := repo.Revoke(context.Background(), 99)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestAccessRepoListOrder(t *testing.T) {
	ctx := context.Background()
	repo := storage.NewAccessRepo(openTestDB(t))

	for _, id := range []int64{30, 10, 20} {
		err := repo.Allow(ctx, id, 1)
		if err != nil {
			t.Fatalf("allow %d: %v", id, err)
		}
	}
	ids, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !slices.Equal(ids, []int64{10, 20, 30}) {
		t.Fatalf("got %v, want [10 20 30]", ids)
	}
}

func TestAccessRepoIsAllowedByUsername(t *testing.T) {
	ctx := context.Background()
	repo := storage.NewAccessRepo(openTestDB(t))

	err := repo.AllowUsername(ctx, "alice", 1)
	if err != nil {
		t.Fatalf("allow username: %v", err)
	}
	ok, err := repo.IsAllowed(ctx, 7, "alice")
	if err != nil || !ok {
		t.Fatalf("IsAllowed(7, alice) = %v, %v; want true", ok, err)
	}
	ok, err = repo.IsAllowed(ctx, 7, "bob")
	if err != nil || ok {
		t.Fatalf("IsAllowed(7, bob) = %v, %v; want false", ok, err)
	}
}

func TestAccessRepoIsAllowedByIDWithEmptyUsername(t *testing.T) {
	ctx := context.Background()
	repo := storage.NewAccessRepo(openTestDB(t))

	err := repo.Allow(ctx, 5, 1)
	if err != nil {
		t.Fatalf("allow: %v", err)
	}
	ok, err := repo.IsAllowed(ctx, 5, "")
	if err != nil || !ok {
		t.Fatalf("IsAllowed(5, \"\") = %v, %v; want true", ok, err)
	}
}

func TestAccessRepoEmptyUsernameNeverMatches(t *testing.T) {
	ctx := context.Background()
	repo := storage.NewAccessRepo(openTestDB(t))

	err := repo.AllowUsername(ctx, "alice", 1)
	if err != nil {
		t.Fatalf("allow username: %v", err)
	}
	ok, err := repo.IsAllowed(ctx, 7, "")
	if err != nil || ok {
		t.Fatalf("IsAllowed(7, \"\") = %v, %v; want false", ok, err)
	}
}

func TestAccessRepoAllowUsernameTwice(t *testing.T) {
	ctx := context.Background()
	repo := storage.NewAccessRepo(openTestDB(t))

	err := repo.AllowUsername(ctx, "alice", 1)
	if err != nil {
		t.Fatalf("first allow: %v", err)
	}
	err = repo.AllowUsername(ctx, "alice", 2)
	if err != nil {
		t.Fatalf("second allow: %v", err)
	}
	names, err := repo.ListUsernames(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !slices.Equal(names, []string{"alice"}) {
		t.Fatalf("got %v, want [alice]", names)
	}
}

func TestAccessRepoRevokeUsername(t *testing.T) {
	ctx := context.Background()
	repo := storage.NewAccessRepo(openTestDB(t))

	err := repo.AllowUsername(ctx, "alice", 1)
	if err != nil {
		t.Fatalf("allow username: %v", err)
	}
	err = repo.RevokeUsername(ctx, "alice")
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	ok, err := repo.IsAllowed(ctx, 7, "alice")
	if err != nil || ok {
		t.Fatalf("IsAllowed after revoke = %v, %v; want false", ok, err)
	}
}

func TestAccessRepoRevokeUsernameUnknown(t *testing.T) {
	repo := storage.NewAccessRepo(openTestDB(t))

	err := repo.RevokeUsername(context.Background(), "ghost")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestAccessRepoListUsernamesOrder(t *testing.T) {
	ctx := context.Background()
	repo := storage.NewAccessRepo(openTestDB(t))

	for _, name := range []string{"carol", "alice", "bob"} {
		err := repo.AllowUsername(ctx, name, 1)
		if err != nil {
			t.Fatalf("allow %s: %v", name, err)
		}
	}
	names, err := repo.ListUsernames(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !slices.Equal(names, []string{"alice", "bob", "carol"}) {
		t.Fatalf("got %v, want [alice bob carol]", names)
	}
}
