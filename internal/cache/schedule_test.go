package cache

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/chivta/knubaschedulebot/internal/domain"
)

const testTTL = time.Minute

// fakeUpstream counts calls so tests can tell a hit from a fetch.
type fakeUpstream struct {
	calls   int
	err     error
	lessons []domain.Lesson
}

func (f *fakeUpstream) Faculties(context.Context) ([]domain.Faculty, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return []domain.Faculty{{ID: 1, Name: "IT"}}, nil
}

func (f *fakeUpstream) Courses(context.Context, int) ([]int, error) {
	f.calls++
	return []int{1, 2}, nil
}

func (f *fakeUpstream) Groups(context.Context, int, int) ([]domain.Group, error) {
	f.calls++
	return []domain.Group{{ID: 7, Name: "KN-21"}}, nil
}

func (f *fakeUpstream) Lessons(context.Context, domain.Group, string, string) ([]domain.Lesson, error) {
	f.calls++
	return f.lessons, nil
}

func newTestCache(t *testing.T) (*Schedule, *fakeUpstream, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	up := &fakeUpstream{}
	return NewSchedule(rdb, up, testTTL), up, mr
}

func TestSecondCallServedFromCache(t *testing.T) {
	ctx := context.Background()
	s, up, _ := newTestCache(t)

	for range 2 {
		got, err := s.Faculties(ctx)
		if err != nil || len(got) != 1 || got[0].Name != "IT" {
			t.Fatalf("got %v, %v", got, err)
		}
	}
	if up.calls != 1 {
		t.Fatalf("upstream calls = %d, want 1", up.calls)
	}
}

func TestExpiryRefetches(t *testing.T) {
	ctx := context.Background()
	s, up, mr := newTestCache(t)

	_, err := s.Courses(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	mr.FastForward(testTTL + time.Second)
	_, err = s.Courses(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if up.calls != 2 {
		t.Fatalf("upstream calls = %d, want 2", up.calls)
	}
}

func TestUpstreamErrorNotCached(t *testing.T) {
	ctx := context.Background()
	s, up, _ := newTestCache(t)
	boom := errors.New("boom")
	up.err = boom

	_, err := s.Faculties(ctx)
	if !errors.Is(err, boom) {
		t.Fatalf("got %v, want boom", err)
	}
	up.err = nil
	got, err := s.Faculties(ctx)
	if err != nil || len(got) != 1 {
		t.Fatalf("got %v, %v", got, err)
	}
	if up.calls != 2 {
		t.Fatalf("upstream calls = %d, want 2", up.calls)
	}
}

func TestRedisDownFallsThrough(t *testing.T) {
	ctx := context.Background()
	s, up, mr := newTestCache(t)
	mr.Close()

	got, err := s.Groups(ctx, 1, 2)
	if err != nil || len(got) != 1 || got[0].ID != 7 {
		t.Fatalf("got %v, %v", got, err)
	}
	if up.calls != 1 {
		t.Fatalf("upstream calls = %d, want 1", up.calls)
	}
}

func TestEmptyLessonsCached(t *testing.T) {
	ctx := context.Background()
	s, up, _ := newTestCache(t)
	group := domain.Group{ID: 7}

	for range 2 {
		got, err := s.Lessons(ctx, group, "2026-10-05", "2026-10-11")
		if err != nil || len(got) != 0 {
			t.Fatalf("got %v, %v", got, err)
		}
	}
	if up.calls != 1 {
		t.Fatalf("upstream calls = %d, want 1", up.calls)
	}
}

func TestLessonsKeyedByWeek(t *testing.T) {
	ctx := context.Background()
	s, up, mr := newTestCache(t)
	group := domain.Group{ID: 7}

	_, err := s.Lessons(ctx, group, "2026-10-05", "2026-10-11")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Lessons(ctx, group, "2026-10-12", "2026-10-18")
	if err != nil {
		t.Fatal(err)
	}
	if up.calls != 2 {
		t.Fatalf("upstream calls = %d, want 2", up.calls)
	}
	if !mr.Exists("mkr:lessons:7:2026-10-05:2026-10-11") || !mr.Exists("mkr:lessons:7:2026-10-12:2026-10-18") {
		t.Fatalf("expected both week keys, got %v", mr.Keys())
	}
}
