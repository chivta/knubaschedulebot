package bot

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
	tele "gopkg.in/telebot.v4"

	"github.com/chivta/knubaschedulebot/internal/domain"
)

// handleStart greets a user: a returning one gets the menu, a new one goes
// straight to picking a group.
func (b *Bot) handleStart(c tele.Context) error {
	group, err := b.users.Group(b.ctx, c.Sender().ID)
	if errors.Is(err, domain.ErrNotFound) {
		// Two messages on purpose. The greeting brings the menu keyboard, which
		// only a new message can deliver. The picker is its own message because
		// it is edited in place from here on, and an edit cannot carry a
		// keyboard of that kind.
		err = b.send(c, textWelcome, mainMenu())
		if err != nil {
			return err
		}

		return b.showFaculties(c, textChooseFaculty)
	}
	if err != nil {
		return b.fail(c, err)
	}

	return b.send(c, fmt.Sprintf(textMenu, escape(group.Name)), mainMenu())
}

func (b *Bot) handleHelp(c tele.Context) error {
	text := textHelp
	if b.isAdmin[c.Sender().ID] {
		text += textHelpAdmin
	}

	return b.send(c, text, mainMenu())
}

// handleUnknown answers free text that is neither a command nor a menu button.
func (b *Bot) handleUnknown(c tele.Context) error {
	return b.send(c, textUnknown, mainMenu())
}

func (b *Bot) handleToday(c tele.Context) error {
	return b.showDay(c, b.today())
}

func (b *Bot) handleTomorrow(c tele.Context) error {
	return b.showDay(c, b.today().AddDate(0, 0, 1))
}

func (b *Bot) handleWeek(c tele.Context) error {
	return b.showWeek(c, monday(b.today()))
}

func (b *Bot) handleNextWeek(c tele.Context) error {
	return b.showWeek(c, monday(b.today()).AddDate(0, 0, daysInWeek))
}

// handleDay opens the day a navigation button points at.
func (b *Bot) handleDay(c tele.Context) error {
	day, err := b.dateArg(c)
	if err != nil {
		return b.fail(c, err)
	}

	return b.showDay(c, day)
}

// handleWeekOf opens the week a navigation button points at.
func (b *Bot) handleWeekOf(c tele.Context) error {
	day, err := b.dateArg(c)
	if err != nil {
		return b.fail(c, err)
	}

	return b.showWeek(c, monday(day))
}

// showDay renders one day of the sender's group.
func (b *Bot) showDay(c tele.Context, day time.Time) error {
	group, lessons, err := b.weekLessons(c, day)
	if errors.Is(err, errNoGroup) {
		return b.showFaculties(c, textGroupFirst)
	}
	if err != nil {
		return b.fail(c, err)
	}

	return b.render(c, dayText(group, day, lessons), dayNav(day))
}

// showWeek renders the week starting at weekStart for the sender's group.
func (b *Bot) showWeek(c tele.Context, weekStart time.Time) error {
	group, lessons, err := b.weekLessons(c, weekStart)
	if errors.Is(err, errNoGroup) {
		return b.showFaculties(c, textGroupFirst)
	}
	if err != nil {
		return b.fail(c, err)
	}

	return b.render(c, weekText(group, weekStart, lessons), weekNav(weekStart))
}

// errNoGroup marks a schedule request from a user who has not picked a group.
// It never leaves this package: the handlers turn it into the group picker.
var errNoGroup = errors.New("no_group")

// weekLessons loads the sender's group and the lessons of the week containing
// day. Asking for whole weeks means "today", "tomorrow" and "this week" share
// one cached response from the schedule site.
func (b *Bot) weekLessons(c tele.Context, day time.Time) (domain.Group, []domain.Lesson, error) {
	group, err := b.users.Group(b.ctx, c.Sender().ID)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.Group{}, nil, errNoGroup
	}
	if err != nil {
		return domain.Group{}, nil, err
	}

	// A cache miss means a round trip to the schedule site, which takes a
	// second or more. The indicator tells the user the tap registered.
	b.typing(c)

	weekStart := monday(day)
	weekEnd := weekStart.AddDate(0, 0, daysInWeek-1)

	lessons, err := b.schedule.Lessons(b.ctx, group,
		weekStart.Format(domain.DateLayout), weekEnd.Format(domain.DateLayout))
	if err != nil {
		return domain.Group{}, nil, err
	}

	return group, lessons, nil
}

// typing shows the "typing" indicator. It is a courtesy, so a failure is not
// worth failing the request over.
func (b *Bot) typing(c tele.Context) {
	err := b.sends.wait(b.ctx)
	if err != nil {
		return
	}

	err = c.Notify(tele.Typing)
	if err != nil {
		log.Debug().Err(err).Msg("failed to send the typing indicator")
	}
}

// dateArg reads the date a navigation button carries.
func (b *Bot) dateArg(c tele.Context) (time.Time, error) {
	args := c.Args()
	if len(args) != 1 {
		return time.Time{}, domain.ErrInvalidInput
	}

	day, err := time.ParseInLocation(domain.DateLayout, args[0], b.location)
	if err != nil {
		return time.Time{}, domain.ErrInvalidInput
	}

	return day, nil
}

// intArgs reads exactly want integers from a button payload.
func intArgs(c tele.Context, want int) ([]int, error) {
	args := c.Args()
	if len(args) != want {
		return nil, domain.ErrInvalidInput
	}

	values := make([]int, 0, want)
	for _, arg := range args {
		value, err := strconv.Atoi(arg)
		if err != nil {
			return nil, domain.ErrInvalidInput
		}
		values = append(values, value)
	}

	return values, nil
}
