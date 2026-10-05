package bot

import (
	"context"
	"errors"
	"fmt"
	"time"

	// The production image is bare Alpine with no zoneinfo, so the binary
	// carries its own copy for the Europe/Kyiv lookup below.
	_ "time/tzdata"

	"github.com/rs/zerolog/log"
	tele "gopkg.in/telebot.v4"

	"github.com/chivta/knubaschedulebot/internal/domain"
	"github.com/chivta/knubaschedulebot/internal/metrics"
)

const (
	// pollerTimeout is how long a long-poll request waits for an update.
	pollerTimeout = 10 * time.Second
	// sendRate and sendBurst keep every outbound call, across all chats,
	// inside Telegram's global 30-per-second ceiling.
	sendRate  = 25
	sendBurst = 5
	// timezone is where the university is. "Today" means today in Kyiv, no
	// matter where the server runs.
	timezone = "Europe/Kyiv"
)

// allowedUpdates are the update kinds the bot handles. Telegram sends nothing
// else, and a kind missing here would arrive as silence rather than an error.
var allowedUpdates = []string{"message", "callback_query"}

// schedule is the schedule site seen from the bot. Declaring it here, in the
// consumer, keeps the bot unaware of whether a cache sits in front of the site.
type schedule interface {
	Faculties(ctx context.Context) ([]domain.Faculty, error)
	Courses(ctx context.Context, facultyID int) ([]int, error)
	Groups(ctx context.Context, facultyID, course int) ([]domain.Group, error)
	Lessons(ctx context.Context, group domain.Group, from, to string) ([]domain.Lesson, error)
}

// userStore remembers which group each user picked.
type userStore interface {
	Group(ctx context.Context, userID int64) (domain.Group, error)
	SetGroup(ctx context.Context, userID int64, group domain.Group) error
}

// accessStore is the allow list that admins edit at runtime.
type accessStore interface {
	IsAllowed(ctx context.Context, userID int64) (bool, error)
	Allow(ctx context.Context, userID, addedBy int64) error
	Revoke(ctx context.Context, userID int64) error
	List(ctx context.Context) ([]int64, error)
}

// Settings is what the bot needs from configuration.
type Settings struct {
	Token string
	// APIURL is the Bot API base URL.
	APIURL string
	// Admins may always use the bot and may edit the allow list.
	Admins []int64
}

// Bot is the Telegram delivery layer. It routes updates, checks access and
// renders what the schedule and the stores return.
type Bot struct {
	bot      *tele.Bot
	schedule schedule
	users    userStore
	access   accessStore
	admins   []int64
	isAdmin  map[int64]bool
	sends    *sendLimiter
	location *time.Location
	// ctx is the lifetime of the bot, set by Run. telebot handlers receive no
	// context of their own, so every call a handler makes hangs off this one.
	ctx context.Context
}

// New builds the bot and checks the token against the Bot API.
func New(settings Settings, source schedule, users userStore, access accessStore) (*Bot, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("load timezone %s: %w", timezone, err)
	}

	inner, err := tele.NewBot(tele.Settings{
		Token:  settings.Token,
		URL:    settings.APIURL,
		Poller: &tele.LongPoller{Timeout: pollerTimeout, AllowedUpdates: allowedUpdates},
		OnError: func(err error, c tele.Context) {
			// Handlers report their own failures to the user and return nil, so
			// what reaches this point is the Bot API refusing a call.
			log.Error().Err(err).Msg("telegram call failed")
		},
	})
	if err != nil {
		return nil, err
	}

	isAdmin := make(map[int64]bool, len(settings.Admins))
	for _, id := range settings.Admins {
		isAdmin[id] = true
	}

	return &Bot{
		bot:      inner,
		schedule: source,
		users:    users,
		access:   access,
		admins:   settings.Admins,
		isAdmin:  isAdmin,
		sends:    newSendLimiter(sendRate, sendBurst),
		location: location,
	}, nil
}

// Run registers the handlers and serves updates until ctx is cancelled.
func (b *Bot) Run(ctx context.Context) error {
	b.ctx = ctx

	// Access is checked before anything else, for every kind of update.
	b.bot.Use(b.requireAccess)

	b.bot.Handle("/start", b.handleStart)
	b.bot.Handle("/help", b.handleHelp)
	b.bot.Handle("/today", b.handleToday)
	b.bot.Handle("/tomorrow", b.handleTomorrow)
	b.bot.Handle("/week", b.handleWeek)
	b.bot.Handle("/nextweek", b.handleNextWeek)
	b.bot.Handle("/group", b.handleChooseGroup)

	b.bot.Handle(btnToday, b.handleToday)
	b.bot.Handle(btnTomorrow, b.handleTomorrow)
	b.bot.Handle(btnWeek, b.handleWeek)
	b.bot.Handle(btnNextWeek, b.handleNextWeek)
	b.bot.Handle(btnGroup, b.handleChooseGroup)
	b.bot.Handle(tele.OnText, b.handleUnknown)

	buttons := b.bot.Group()
	buttons.Use(b.acknowledge)
	buttons.Handle(&tele.Btn{Unique: cbFaculties}, b.handleChooseGroup)
	buttons.Handle(&tele.Btn{Unique: cbFaculty}, b.handleFaculty)
	buttons.Handle(&tele.Btn{Unique: cbCourse}, b.handleCourse)
	buttons.Handle(&tele.Btn{Unique: cbGroup}, b.handleGroup)
	buttons.Handle(&tele.Btn{Unique: cbDay}, b.handleDay)
	buttons.Handle(&tele.Btn{Unique: cbToday}, b.handleToday)
	buttons.Handle(&tele.Btn{Unique: cbWeek}, b.handleWeekOf)
	buttons.Handle(&tele.Btn{Unique: cbThisWeek}, b.handleWeek)

	admin := b.bot.Group()
	admin.Use(b.requireAdmin)
	admin.Handle("/allow", b.handleAllow)
	admin.Handle("/deny", b.handleDeny)
	admin.Handle("/users", b.handleUsers)

	err := b.bot.SetCommands([]tele.Command{
		{Text: "today", Description: "пари на сьогодні"},
		{Text: "tomorrow", Description: "пари на завтра"},
		{Text: "week", Description: "поточний тиждень"},
		{Text: "nextweek", Description: "наступний тиждень"},
		{Text: "group", Description: "змінити групу"},
		{Text: "help", Description: "довідка"},
	})
	if err != nil {
		// Not fatal: the bot works fine, the command menu is just not published.
		log.Warn().Err(err).Msg("failed to publish bot commands")
	}

	// telebot's Start blocks and has no context-aware variant, so cancellation
	// is bridged onto Stop.
	go func() {
		<-ctx.Done()
		b.bot.Stop()
	}()

	log.Info().Str("username", b.bot.Me.Username).Msg("telegram bot listening")
	b.bot.Start()
	log.Info().Msg("telegram bot stopped")

	return nil
}

// requireAccess lets an update through only when its sender is an admin or on
// the allow list. Everyone else is told whom to ask.
func (b *Bot) requireAccess(next tele.HandlerFunc) tele.HandlerFunc {
	return func(c tele.Context) error {
		sender := c.Sender()
		if sender == nil {
			// Channel posts and the like have nobody to authorise or to answer.
			return nil
		}

		allowed, err := b.allows(sender.ID)
		if err != nil {
			return b.fail(c, err)
		}
		if !allowed {
			metrics.IncDenied()
			log.Info().Int64("user_id", sender.ID).Msg("denied a user outside the allow list")

			return b.deny(c)
		}

		metrics.IncRequest()

		return next(c)
	}
}

// requireAdmin guards the commands that edit the allow list.
func (b *Bot) requireAdmin(next tele.HandlerFunc) tele.HandlerFunc {
	return func(c tele.Context) error {
		if !b.isAdmin[c.Sender().ID] {
			return b.send(c, describe(domain.ErrForbidden))
		}

		return next(c)
	}
}

// acknowledge answers a button tap before the work starts. Without an answer
// the user's client shows a spinner on the button, and a tap that waits on a
// slow schedule site would outlive the time Telegram allows for answering.
func (b *Bot) acknowledge(next tele.HandlerFunc) tele.HandlerFunc {
	return func(c tele.Context) error {
		if c.Callback() != nil {
			err := b.respond(c)
			if err != nil {
				log.Debug().Err(err).Msg("failed to answer a button tap")
			}
		}

		return next(c)
	}
}

// allows reports whether a user may use the bot. Admins come from
// configuration and are never looked up in the store.
func (b *Bot) allows(userID int64) (bool, error) {
	if b.isAdmin[userID] {
		return true, nil
	}

	return b.access.IsAllowed(b.ctx, userID)
}

// deny tells a user outside the allow list whom to write to. Their ID is in
// the message because that is what an admin needs to add them.
func (b *Bot) deny(c tele.Context) error {
	if c.Callback() != nil {
		return b.respond(c, &tele.CallbackResponse{
			Text:      fmt.Sprintf(textDeniedAlert, supportContact),
			ShowAlert: true,
		})
	}

	return b.send(c, fmt.Sprintf(textDenied, supportContact, c.Sender().ID))
}

// fail tells the user what went wrong and logs it, once. The handler returns
// whatever this returns, so a failure the user caused never reaches OnError.
func (b *Bot) fail(c tele.Context, cause error) error {
	event := log.Error()
	if errors.Is(cause, domain.ErrNotFound) || errors.Is(cause, domain.ErrInvalidInput) {
		// The user asked for something that does not exist. Worth a trace,
		// not an alert.
		event = log.Info()
	}
	event.Err(cause).Int64("user_id", c.Sender().ID).Msg("request failed")

	return b.send(c, describe(cause))
}

// render shows text in place when the update is a button tap, and as a new
// message otherwise.
func (b *Bot) render(c tele.Context, text string, markup *tele.ReplyMarkup) error {
	if c.Callback() == nil {
		return b.send(c, text, markup)
	}

	err := b.sends.wait(b.ctx)
	if err != nil {
		return err
	}

	err = c.Edit(text, markup, tele.ModeHTML, tele.NoPreview)
	if errors.Is(err, tele.ErrSameMessageContent) || errors.Is(err, tele.ErrMessageNotModified) {
		// The user tapped a button that leads to what is already on screen.
		return nil
	}

	return err
}

// send posts a new message to the chat the update came from.
func (b *Bot) send(c tele.Context, text string, opts ...any) error {
	err := b.sends.wait(b.ctx)
	if err != nil {
		return err
	}

	return c.Send(text, append(opts, tele.ModeHTML, tele.NoPreview)...)
}

// respond answers a button tap, optionally with a popup.
func (b *Bot) respond(c tele.Context, response ...*tele.CallbackResponse) error {
	err := b.sends.wait(b.ctx)
	if err != nil {
		return err
	}

	return c.Respond(response...)
}

// today is the current calendar day in Kyiv, at midnight.
func (b *Bot) today() time.Time {
	now := time.Now().In(b.location)

	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, b.location)
}
