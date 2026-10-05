package bot

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
	tele "gopkg.in/telebot.v4"

	"github.com/chivta/knubaschedulebot/internal/domain"
)

// usernamePrefix is how a username is written in a chat.
const usernamePrefix = "@"

// usernamePattern is the shape of a normalised Telegram username: it starts
// with a letter and is 4 to 32 characters long.
var usernamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{3,31}$`)

// accessTarget is whom an admin command points at. Exactly one field is set.
type accessTarget struct {
	userID   int64
	username string
}

// handleAllow adds a user to the allow list: /allow <telegram id or @username>.
func (b *Bot) handleAllow(c tele.Context) error {
	target, err := targetArg(c)
	if err != nil {
		return b.send(c, textAllowUsage)
	}
	if target.username != "" {
		return b.allowUsername(c, target.username)
	}
	if b.isAdmin[target.userID] {
		return b.send(c, fmt.Sprintf(textIsAdmin, target.userID))
	}

	err = b.access.Allow(b.ctx, target.userID, c.Sender().ID)
	if err != nil {
		return b.fail(c, err)
	}

	log.Info().Int64("user_id", target.userID).Int64("admin_id", c.Sender().ID).Msg("user added to the allow list")

	b.announceAccess(target.userID)

	return b.send(c, fmt.Sprintf(textAllowed, target.userID))
}

// allowUsername adds a username to the allow list. Whoever holds the username
// gets in, and nobody is told: the Bot API cannot message a username.
func (b *Bot) allowUsername(c tele.Context, username string) error {
	err := b.access.AllowUsername(b.ctx, username, c.Sender().ID)
	if err != nil {
		return b.fail(c, err)
	}

	log.Info().Str("username", username).Int64("admin_id", c.Sender().ID).Msg("username added to the allow list")

	return b.send(c, fmt.Sprintf(textAllowedUsername, username))
}

// handleDeny removes a user from the allow list: /deny <telegram id or @username>.
func (b *Bot) handleDeny(c tele.Context) error {
	target, err := targetArg(c)
	if err != nil {
		return b.send(c, textDenyUsage)
	}
	if target.username != "" {
		return b.denyUsername(c, target.username)
	}
	if b.isAdmin[target.userID] {
		return b.send(c, fmt.Sprintf(textIsAdmin, target.userID))
	}

	err = b.access.Revoke(b.ctx, target.userID)
	if errors.Is(err, domain.ErrNotFound) {
		return b.send(c, fmt.Sprintf(textNotOnList, target.userID))
	}
	if err != nil {
		return b.fail(c, err)
	}

	log.Info().Int64("user_id", target.userID).Int64("admin_id", c.Sender().ID).Msg("user removed from the allow list")

	return b.send(c, fmt.Sprintf(textRevoked, target.userID))
}

// denyUsername removes a username from the allow list.
func (b *Bot) denyUsername(c tele.Context, username string) error {
	err := b.access.RevokeUsername(b.ctx, username)
	if errors.Is(err, domain.ErrNotFound) {
		return b.send(c, fmt.Sprintf(textUsernameNotOnList, username))
	}
	if err != nil {
		return b.fail(c, err)
	}

	log.Info().Str("username", username).Int64("admin_id", c.Sender().ID).Msg("username removed from the allow list")

	return b.send(c, fmt.Sprintf(textRevokedUsername, username))
}

// handleUsers lists the admins and everyone on the allow list.
func (b *Bot) handleUsers(c tele.Context) error {
	userIDs, err := b.access.List(b.ctx)
	if err != nil {
		return b.fail(c, err)
	}

	usernames, err := b.access.ListUsernames(b.ctx)
	if err != nil {
		return b.fail(c, err)
	}

	allowed := idLines(userIDs)
	for _, username := range usernames {
		allowed = append(allowed, fmt.Sprintf(textUsernameLine, username))
	}

	text := textUsersAdmins + "\n" + listText(idLines(b.admins)) +
		"\n\n" + textUsersAllowed + "\n" + listText(allowed)

	return b.send(c, text)
}

// announceAccess tells a newly allowed user they can start. They have usually
// messaged the bot already, since that is how they learned their ID. If they
// have not, Telegram refuses the message and there is nothing to do about it.
func (b *Bot) announceAccess(userID int64) {
	err := b.sends.wait(b.ctx)
	if err != nil {
		return
	}

	_, err = b.bot.Send(&tele.User{ID: userID}, textAccessGranted, tele.ModeHTML)
	if err != nil {
		log.Debug().Err(err).Int64("user_id", userID).Msg("failed to announce access to the user")
	}
}

// targetArg reads the single user an admin command takes: a number is a
// Telegram ID, anything else has to be a username.
func targetArg(c tele.Context) (accessTarget, error) {
	args := c.Args()
	if len(args) != 1 {
		return accessTarget{}, domain.ErrInvalidInput
	}

	userID, err := strconv.ParseInt(args[0], 10, 64)
	if err == nil {
		if userID <= 0 {
			return accessTarget{}, domain.ErrInvalidInput
		}

		return accessTarget{userID: userID}, nil
	}

	username := normalizeUsername(args[0])
	if !usernamePattern.MatchString(username) {
		return accessTarget{}, domain.ErrInvalidInput
	}

	return accessTarget{username: username}, nil
}

// normalizeUsername brings a username to the form the allow list stores:
// lowercase, without the leading "@". Telegram usernames are case-insensitive.
func normalizeUsername(username string) string {
	return strings.ToLower(strings.TrimPrefix(username, usernamePrefix))
}

// idLines renders user IDs, one entry per ID.
func idLines(ids []int64) []string {
	lines := make([]string, 0, len(ids))
	for _, id := range ids {
		lines = append(lines, fmt.Sprintf(textUserLine, id))
	}

	return lines
}

// listText renders entries one per line, or says the list is empty.
func listText(lines []string) string {
	if len(lines) == 0 {
		return textUsersNone
	}

	return strings.Join(lines, "\n")
}
