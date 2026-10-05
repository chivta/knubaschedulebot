package bot

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
	tele "gopkg.in/telebot.v4"

	"github.com/chivta/knubaschedulebot/internal/domain"
)

// handleAllow adds a user to the allow list: /allow <telegram id>.
func (b *Bot) handleAllow(c tele.Context) error {
	userID, err := userIDArg(c)
	if err != nil {
		return b.send(c, textAllowUsage)
	}
	if b.isAdmin[userID] {
		return b.send(c, fmt.Sprintf(textIsAdmin, userID))
	}

	err = b.access.Allow(b.ctx, userID, c.Sender().ID)
	if err != nil {
		return b.fail(c, err)
	}

	log.Info().Int64("user_id", userID).Int64("admin_id", c.Sender().ID).Msg("user added to the allow list")

	b.announceAccess(userID)

	return b.send(c, fmt.Sprintf(textAllowed, userID))
}

// handleDeny removes a user from the allow list: /deny <telegram id>.
func (b *Bot) handleDeny(c tele.Context) error {
	userID, err := userIDArg(c)
	if err != nil {
		return b.send(c, textDenyUsage)
	}
	if b.isAdmin[userID] {
		return b.send(c, fmt.Sprintf(textIsAdmin, userID))
	}

	err = b.access.Revoke(b.ctx, userID)
	if errors.Is(err, domain.ErrNotFound) {
		return b.send(c, fmt.Sprintf(textNotOnList, userID))
	}
	if err != nil {
		return b.fail(c, err)
	}

	log.Info().Int64("user_id", userID).Int64("admin_id", c.Sender().ID).Msg("user removed from the allow list")

	return b.send(c, fmt.Sprintf(textRevoked, userID))
}

// handleUsers lists the admins and everyone on the allow list.
func (b *Bot) handleUsers(c tele.Context) error {
	allowed, err := b.access.List(b.ctx)
	if err != nil {
		return b.fail(c, err)
	}

	text := textUsersAdmins + "\n" + idLines(b.admins) +
		"\n\n" + textUsersAllowed + "\n" + idLines(allowed)

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

// userIDArg reads the single Telegram user ID an admin command takes.
func userIDArg(c tele.Context) (int64, error) {
	args := c.Args()
	if len(args) != 1 {
		return 0, domain.ErrInvalidInput
	}

	userID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil || userID <= 0 {
		return 0, domain.ErrInvalidInput
	}

	return userID, nil
}

// idLines renders user IDs one per line.
func idLines(ids []int64) string {
	if len(ids) == 0 {
		return textUsersNone
	}

	lines := make([]string, 0, len(ids))
	for _, id := range ids {
		lines = append(lines, fmt.Sprintf(textUserLine, id))
	}

	return strings.Join(lines, "\n")
}
