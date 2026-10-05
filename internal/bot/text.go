package bot

import (
	"errors"
	"strings"

	"github.com/chivta/knubaschedulebot/internal/domain"
)

// Every user-facing string lives here. Nothing in the other layers formats text
// for a human: they pass data and domain errors, and this file names them.
// Keeping it in one file is what makes the bot translatable later, and what
// stops HTML-escaping bugs from spreading through the handlers.

// supportContact is who a user outside the allow list is sent to.
const supportContact = "@ukbotsup"

// Reply keyboard buttons. Telegram sends the button's text back as a plain
// message, so these strings are also the routing keys of their handlers.
const (
	btnToday    = "📅 Сьогодні"
	btnTomorrow = "➡️ Завтра"
	btnWeek     = "🗓 Тиждень"
	btnNextWeek = "⏭ Наступний тиждень"
	btnGroup    = "👥 Змінити групу"
)

// Inline button labels.
const (
	labelBack        = "« Назад"
	labelToday       = "Сьогодні"
	labelPrevDay     = "◀ %s"
	labelNextDay     = "%s ▶"
	labelPrevWeek    = "◀ Попередній"
	labelCurrentWeek = "Поточний"
	labelNextWeek    = "Наступний ▶"
	labelCourse      = "%d курс"
)

const (
	textWelcome = "👋 Вітаю! Я показую розклад КНУБА з mkr.knuba.edu.ua.\n\n" +
		"Спершу оберіть свій факультет."
	textChooseFaculty = "Оберіть факультет."
	textChooseCourse  = "Оберіть курс."
	textChooseGroup   = "Оберіть групу."
	textGroupFirst    = "Спершу оберіть групу. Який у вас факультет?"
	// textGroupSaved and textMenu take the group name.
	textGroupSaved = "✅ Групу <b>%s</b> збережено.\n\nРозклад відкривають кнопки нижче."
	textMenu       = "Ваша група: <b>%s</b>.\n\nРозклад відкривають кнопки нижче."
	textUnknown    = "Не розумію це повідомлення. Скористайтеся кнопками нижче або командою /help."

	textHelp = "<b>Розклад КНУБА</b>\n\n" +
		"/today пари на сьогодні\n" +
		"/tomorrow пари на завтра\n" +
		"/week поточний тиждень\n" +
		"/nextweek наступний тиждень\n" +
		"/group змінити групу\n\n" +
		"Розклад береться з mkr.knuba.edu.ua й оновлюється раз на годину."
	textHelpAdmin = "\n\n<b>Адміністрування</b>\n" +
		"/allow <code>ID</code> або <code>@тег</code> додати користувача до білого списку\n" +
		"/deny <code>ID</code> або <code>@тег</code> прибрати користувача\n" +
		"/users показати білий список"

	// textDenied takes the support contact and the sender's Telegram ID, which
	// is what an admin needs to add them.
	textDenied = "⛔ У вас немає доступу до цього бота.\n\n" +
		"Напишіть %s, щоб потрапити до білого списку.\n" +
		"Ваш ID: <code>%d</code>"
	// textDeniedAlert is the popup shown when a denied user taps a button.
	textDeniedAlert = "Немає доступу. Напишіть %s."

	textAllowUsage = "Вкажіть ID або тег користувача, наприклад " +
		"<code>/allow 123456789</code> чи <code>/allow @username</code>"
	textDenyUsage = "Вкажіть ID або тег користувача, наприклад " +
		"<code>/deny 123456789</code> чи <code>/deny @username</code>"
	// The admin texts below take a Telegram user ID, or a username without
	// the "@" for the ones named after it.
	textAllowed           = "✅ Користувача <code>%d</code> додано до білого списку."
	textRevoked           = "🚫 Користувача <code>%d</code> прибрано з білого списку."
	textNotOnList         = "Користувача <code>%d</code> немає в білому списку."
	textAllowedUsername   = "✅ Користувача @%s додано до білого списку."
	textRevokedUsername   = "🚫 Користувача @%s прибрано з білого списку."
	textUsernameNotOnList = "Користувача @%s немає в білому списку."
	textUsernameLine      = "@%s"
	textIsAdmin           = "Користувач <code>%d</code> є адміністратором. Адміністраторів задає конфігурація бота."
	textAccessGranted     = "✅ Вам надано доступ до бота. Натисніть /start."
	textUsersAdmins       = "<b>Адміністратори</b>"
	textUsersAllowed      = "<b>Білий список</b>"
	textUsersNone         = "порожньо"
	textUserLine          = "<code>%d</code>"
	textDayHeader         = "📅 <b>%s, %d %s</b> · %s"
	textDayEmpty          = "Пар немає 🎉"
	textLessonHeader      = "<b>%d пара</b> · %s-%s"
	textLessonSubject     = "%s <i>(%s)</i>"
	textLessonRoom        = "📍 %s"
	textLessonGroups      = "👥 %s"
	textDetailSeparator   = " · "
	textWeekHeader        = "🗓 <b>Тиждень %s-%s</b> · %s"
	textWeekEmpty         = "На цьому тижні пар немає 🎉"
	textWeekDay           = "<b>%s, %s</b>"
	textWeekLesson        = "%d. %s %s"
	textWeekTruncated     = "… Решта днів не вмістилася. Відкрийте їх окремо кнопкою «Сьогодні»."
)

// shortDateLayout is the day.month form used in buttons and week headers.
const shortDateLayout = "02.01"

// Ukrainian calendar words, indexed by time.Weekday and time.Month. Months are
// in the genitive, as they follow a day number.
var (
	weekdayNames = [...]string{"Неділя", "Понеділок", "Вівторок", "Середа", "Четвер", "П'ятниця", "Субота"}
	weekdayShort = [...]string{"Нд", "Пн", "Вт", "Ср", "Чт", "Пт", "Сб"}
	monthNames   = [...]string{
		"", "січня", "лютого", "березня", "квітня", "травня", "червня",
		"липня", "серпня", "вересня", "жовтня", "листопада", "грудня",
	}
)

// errorText maps every domain error onto the one sentence the user gets. An
// error missing from this map is a bug on our side, not theirs, so the
// fallback apologises rather than leaking the error.
var errorText = map[error]string{
	domain.ErrNotFound:     "🤷 Не знайшов такого. Спробуйте обрати ще раз.",
	domain.ErrInvalidInput: "🤔 Не вдалося розібрати запит. Спробуйте ще раз.",
	domain.ErrForbidden:    "🔒 Ця команда лише для адміністраторів.",
	domain.ErrUpstream:     "🌧 Сайт розкладу зараз не відповідає. Спробуйте за кілька хвилин.",
}

const textUnexpected = "💥 Щось пішло не так з мого боку. Спробуйте ще раз."

// describe names an error for the user, falling back to the generic apology.
func describe(err error) string {
	for sentinel, text := range errorText {
		if errors.Is(err, sentinel) {
			return text
		}
	}

	return textUnexpected
}

// escape makes a value safe for tele.ModeHTML. Every interpolated value goes
// through it: one raw "<" in a subject makes Telegram reject the whole message.
func escape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}
