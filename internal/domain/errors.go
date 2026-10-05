package domain

import "errors"

// Sentinel errors describing every condition the bot knows how to explain to a
// user. Anything else surfaces as a generic failure, which is deliberate: an
// unmapped error is a bug on our side, not something to leak verbatim into a
// chat. Keep the values machine-readable — the human sentence lives in
// internal/bot/text.go.
var (
	// ErrNotFound is returned when a lookup yields nothing usable.
	ErrNotFound = errors.New("not_found")
	// ErrInvalidInput is returned when the user's message cannot be understood.
	ErrInvalidInput = errors.New("invalid_input")
	// ErrUnauthorized is returned when a user is not on the allow list.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrForbidden is returned when an allowed user calls an admin command.
	ErrForbidden = errors.New("forbidden")
	// ErrUpstream is returned when the schedule site fails or answers with
	// something the parser does not recognise.
	ErrUpstream = errors.New("upstream")
)
