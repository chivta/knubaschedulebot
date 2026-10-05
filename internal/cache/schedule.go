package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"

	"github.com/chivta/knubaschedulebot/internal/domain"
	"github.com/chivta/knubaschedulebot/internal/metrics"
)

// Key formats. Everything is prefixed so the bot can share a Redis instance.
const (
	keyFaculties = "mkr:faculties"
	keyCourses   = "mkr:courses:%d"
	keyGroups    = "mkr:groups:%d:%d"
	keyLessons   = "mkr:lessons:%d:%s:%s"
)

// upstream is the schedule site as the cache sees it. *mkr.Client satisfies it.
type upstream interface {
	Faculties(ctx context.Context) ([]domain.Faculty, error)
	Courses(ctx context.Context, facultyID int) ([]int, error)
	Groups(ctx context.Context, facultyID, course int) ([]domain.Group, error)
	Lessons(ctx context.Context, group domain.Group, from, to string) ([]domain.Lesson, error)
}

// Schedule is a read-through Redis cache in front of the schedule site. It has
// the same methods as upstream, so callers cannot tell the difference. Redis
// failures never fail a call, they only cost a trip to the site.
type Schedule struct {
	rdb    *redis.Client
	source upstream
	ttl    time.Duration
}

// NewSchedule returns a Schedule that keeps every value for ttl.
func NewSchedule(rdb *redis.Client, source upstream, ttl time.Duration) *Schedule {
	return &Schedule{rdb: rdb, source: source, ttl: ttl}
}

// Faculties returns the faculty list.
func (s *Schedule) Faculties(ctx context.Context) ([]domain.Faculty, error) {
	return remember(ctx, s, keyFaculties, func() ([]domain.Faculty, error) {
		return s.source.Faculties(ctx)
	})
}

// Courses returns the course numbers of a faculty.
func (s *Schedule) Courses(ctx context.Context, facultyID int) ([]int, error) {
	return remember(ctx, s, fmt.Sprintf(keyCourses, facultyID), func() ([]int, error) {
		return s.source.Courses(ctx, facultyID)
	})
}

// Groups returns the groups of one faculty course.
func (s *Schedule) Groups(ctx context.Context, facultyID, course int) ([]domain.Group, error) {
	return remember(ctx, s, fmt.Sprintf(keyGroups, facultyID, course), func() ([]domain.Group, error) {
		return s.source.Groups(ctx, facultyID, course)
	})
}

// Lessons returns the group's lessons between the from and to dates.
func (s *Schedule) Lessons(ctx context.Context, group domain.Group, from, to string) ([]domain.Lesson, error) {
	return remember(ctx, s, fmt.Sprintf(keyLessons, group.ID, from, to), func() ([]domain.Lesson, error) {
		return s.source.Lessons(ctx, group, from, to)
	})
}

// remember returns the value cached under key, or calls fetch and caches its
// result. A hit is any existing key, so an empty result is cached like any
// other. Upstream errors are returned as is and never cached.
func remember[T any](ctx context.Context, s *Schedule, key string, fetch func() (T, error)) (T, error) {
	var value T

	raw, err := s.rdb.Get(ctx, key).Bytes()
	switch {
	case err == nil:
		err = json.Unmarshal(raw, &value)
		if err == nil {
			metrics.IncCacheHit()
			return value, nil
		}
		log.Warn().Err(err).Str("key", key).Msg("cache: undecodable value, refetching")
	case !errors.Is(err, redis.Nil):
		log.Warn().Err(err).Str("key", key).Msg("cache: get failed, falling through to upstream")
	}

	metrics.IncCacheMiss()
	value, err = fetch()
	if err != nil {
		var zero T
		return zero, err
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		log.Warn().Err(err).Str("key", key).Msg("cache: encode failed, value not cached")
		return value, nil
	}
	err = s.rdb.Set(ctx, key, encoded, s.ttl).Err()
	if err != nil {
		log.Warn().Err(err).Str("key", key).Msg("cache: set failed, value not cached")
	}
	return value, nil
}
