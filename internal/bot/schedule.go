package bot

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/chivta/knubaschedulebot/internal/domain"
)

const (
	// daysInWeek is the span of one schedule request: Monday through Sunday.
	daysInWeek = 7
	// weekTextLimit keeps a week under Telegram's 4096-character message cap,
	// with headroom because Telegram counts some emoji as two characters.
	weekTextLimit = 3800
)

// monday returns the Monday of the week containing day. The schedule is always
// requested in whole weeks, so every day of one week shares a cache entry.
func monday(day time.Time) time.Time {
	sinceMonday := (int(day.Weekday()) + daysInWeek - 1) % daysInWeek

	return day.AddDate(0, 0, -sinceMonday)
}

// onDay filters a week of lessons down to one calendar day.
func onDay(lessons []domain.Lesson, day time.Time) []domain.Lesson {
	date := day.Format(domain.DateLayout)

	var found []domain.Lesson
	for _, lesson := range lessons {
		if lesson.Date == date {
			found = append(found, lesson)
		}
	}

	return found
}

// dayText renders one day in full: time, subject, room, teacher.
func dayText(group domain.Group, day time.Time, lessons []domain.Lesson) string {
	var text strings.Builder

	fmt.Fprintf(&text, textDayHeader,
		weekdayNames[day.Weekday()], day.Day(), monthNames[day.Month()], escape(group.Name))

	today := onDay(lessons, day)
	if len(today) == 0 {
		text.WriteString("\n\n" + textDayEmpty)

		return text.String()
	}

	for _, lesson := range today {
		text.WriteString("\n\n")
		fmt.Fprintf(&text, textLessonHeader, lesson.Number, lesson.Start, lesson.End)
		text.WriteString("\n")
		text.WriteString(subjectText(lesson))

		details := lessonDetails(lesson)
		if details != "" {
			text.WriteString("\n" + details)
		}

		// The site names who attends. It only matters when that is not the
		// whole group: a subgroup, or a stream shared with other groups.
		if lesson.Groups != "" && lesson.Groups != group.Name {
			text.WriteString("\n")
			fmt.Fprintf(&text, textLessonGroups, escape(lesson.Groups))
		}
	}

	return text.String()
}

// weekText renders a week compactly, one line per lesson, skipping days
// without classes. Teachers are left to the day view to keep the message short.
func weekText(group domain.Group, weekStart time.Time, lessons []domain.Lesson) string {
	var text strings.Builder

	weekEnd := weekStart.AddDate(0, 0, daysInWeek-1)
	fmt.Fprintf(&text, textWeekHeader,
		weekStart.Format(shortDateLayout), weekEnd.Format(shortDateLayout), escape(group.Name))

	if len(lessons) == 0 {
		text.WriteString("\n\n" + textWeekEmpty)

		return text.String()
	}

	for offset := range daysInWeek {
		day := weekStart.AddDate(0, 0, offset)

		block := weekDayBlock(day, onDay(lessons, day))
		if block == "" {
			continue
		}

		// A day is appended whole or not at all, so a cut never lands inside
		// an HTML tag.
		if utf8.RuneCountInString(text.String())+utf8.RuneCountInString(block) > weekTextLimit {
			text.WriteString("\n\n" + textWeekTruncated)

			break
		}

		text.WriteString(block)
	}

	return text.String()
}

// weekDayBlock renders one day of the week view, or "" when it has no classes.
func weekDayBlock(day time.Time, lessons []domain.Lesson) string {
	if len(lessons) == 0 {
		return ""
	}

	var block strings.Builder

	block.WriteString("\n\n")
	fmt.Fprintf(&block, textWeekDay, weekdayNames[day.Weekday()], day.Format(shortDateLayout))

	for _, lesson := range lessons {
		block.WriteString("\n")
		fmt.Fprintf(&block, textWeekLesson, lesson.Number, lesson.Start, subjectText(lesson))

		if lesson.Room != "" {
			block.WriteString(textDetailSeparator + escape(lesson.Room))
		}
	}

	return block.String()
}

// subjectText is the subject with its class type, when the site gave one.
func subjectText(lesson domain.Lesson) string {
	if lesson.Kind == "" {
		return escape(lesson.Subject)
	}

	return fmt.Sprintf(textLessonSubject, escape(lesson.Subject), escape(lesson.Kind))
}

// lessonDetails joins the room and the teacher, leaving out whichever the site
// did not provide.
func lessonDetails(lesson domain.Lesson) string {
	var parts []string

	if lesson.Room != "" {
		parts = append(parts, fmt.Sprintf(textLessonRoom, escape(lesson.Room)))
	}
	if lesson.Teacher != "" {
		parts = append(parts, escape(lesson.Teacher))
	}

	return strings.Join(parts, textDetailSeparator)
}

// shortDay labels a navigation button, such as "Вт 06.10".
func shortDay(day time.Time) string {
	return weekdayShort[day.Weekday()] + " " + day.Format(shortDateLayout)
}
