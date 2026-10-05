package bot

import (
	"fmt"
	"strconv"
	"time"

	tele "gopkg.in/telebot.v4"

	"github.com/chivta/knubaschedulebot/internal/domain"
)

// Callback names of the inline buttons. telebot routes a tap by this name and
// hands the rest of the payload to the handler as arguments.
const (
	// cbFaculties reopens the faculty list.
	cbFaculties = "faculties"
	// cbFaculty carries a faculty ID and opens its courses.
	cbFaculty = "faculty"
	// cbCourse carries a faculty ID and a course and opens the groups.
	cbCourse = "course"
	// cbGroup carries a faculty ID, a course and a group ID and saves the group.
	cbGroup = "group"
	// cbDay carries a date in domain.DateLayout and opens that day.
	cbDay = "day"
	// cbToday opens the current day, resolved when tapped rather than when the
	// button was drawn, so an old message still jumps to the real today.
	cbToday = "today"
	// cbTomorrow opens the day after the current one, resolved when tapped.
	cbTomorrow = "tomorrow"
	// cbWeek carries a Monday in domain.DateLayout and opens that week.
	cbWeek = "week"
	// cbThisWeek opens the current week, resolved when tapped.
	cbThisWeek = "thisweek"
	// cbNextWeek opens the week after the current one, resolved when tapped.
	cbNextWeek = "nextweek"
)

// groupsPerRow is how many group buttons share a row. Group names are short,
// and three across keeps a forty-group course on one screen.
const groupsPerRow = 3

// mainMenu is the reply keyboard that stays under the input field.
func mainMenu() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{ResizeKeyboard: true, IsPersistent: true}
	menu.Reply(
		menu.Row(menu.Text(btnToday), menu.Text(btnTomorrow)),
		menu.Row(menu.Text(btnWeek), menu.Text(btnNextWeek)),
		menu.Row(menu.Text(btnGroup)),
	)

	return menu
}

// scheduleMenu is the inline counterpart of mainMenu. It replaces the group
// list once a group is saved, so the picker message turns into the way in.
func scheduleMenu() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data(btnToday, cbToday), menu.Data(btnTomorrow, cbTomorrow)),
		menu.Row(menu.Data(btnWeek, cbThisWeek), menu.Data(btnNextWeek, cbNextWeek)),
	)

	return menu
}

// dayNav is the row under a day: previous day, today, next day.
func dayNav(day time.Time) *tele.ReplyMarkup {
	previous := day.AddDate(0, 0, -1)
	next := day.AddDate(0, 0, 1)

	nav := &tele.ReplyMarkup{}
	nav.Inline(nav.Row(
		nav.Data(fmt.Sprintf(labelPrevDay, shortDay(previous)), cbDay, previous.Format(domain.DateLayout)),
		nav.Data(labelToday, cbToday),
		nav.Data(fmt.Sprintf(labelNextDay, shortDay(next)), cbDay, next.Format(domain.DateLayout)),
	))

	return nav
}

// weekNav is the row under a week: previous week, current week, next week.
func weekNav(weekStart time.Time) *tele.ReplyMarkup {
	previous := weekStart.AddDate(0, 0, -daysInWeek)
	next := weekStart.AddDate(0, 0, daysInWeek)

	nav := &tele.ReplyMarkup{}
	nav.Inline(nav.Row(
		nav.Data(labelPrevWeek, cbWeek, previous.Format(domain.DateLayout)),
		nav.Data(labelCurrentWeek, cbThisWeek),
		nav.Data(labelNextWeek, cbWeek, next.Format(domain.DateLayout)),
	))

	return nav
}

// facultyKeyboard lists faculties one per row, since their names are long.
func facultyKeyboard(faculties []domain.Faculty) *tele.ReplyMarkup {
	keyboard := &tele.ReplyMarkup{}

	rows := make([]tele.Row, 0, len(faculties))
	for _, faculty := range faculties {
		rows = append(rows, keyboard.Row(keyboard.Data(faculty.Name, cbFaculty, strconv.Itoa(faculty.ID))))
	}
	keyboard.Inline(rows...)

	return keyboard
}

// courseKeyboard lists the courses of a faculty on one row.
func courseKeyboard(facultyID int, courses []int) *tele.ReplyMarkup {
	keyboard := &tele.ReplyMarkup{}

	buttons := make([]tele.Btn, 0, len(courses))
	for _, course := range courses {
		buttons = append(buttons, keyboard.Data(
			fmt.Sprintf(labelCourse, course), cbCourse, strconv.Itoa(facultyID), strconv.Itoa(course)))
	}

	keyboard.Inline(
		keyboard.Row(buttons...),
		keyboard.Row(keyboard.Data(labelBack, cbFaculties)),
	)

	return keyboard
}

// groupKeyboard lists the groups of one course. The group name is not in the
// payload, which is capped at 64 bytes: the handler looks it up by ID.
func groupKeyboard(facultyID, course int, groups []domain.Group) *tele.ReplyMarkup {
	keyboard := &tele.ReplyMarkup{}

	buttons := make([]tele.Btn, 0, len(groups))
	for _, group := range groups {
		buttons = append(buttons, keyboard.Data(
			group.Name, cbGroup, strconv.Itoa(facultyID), strconv.Itoa(course), strconv.Itoa(group.ID)))
	}

	rows := keyboard.Split(groupsPerRow, buttons)
	rows = append(rows, keyboard.Row(keyboard.Data(labelBack, cbFaculty, strconv.Itoa(facultyID))))
	keyboard.Inline(rows...)

	return keyboard
}
