package bot

import (
	"strings"
	"testing"
	"time"

	"github.com/chivta/knubaschedulebot/internal/domain"
)

var testGroup = domain.Group{ID: 2323, Name: "КН-25-1", FacultyID: 6, Course: 2}

func date(t *testing.T, value string) time.Time {
	t.Helper()

	day, err := time.Parse(domain.DateLayout, value)
	if err != nil {
		t.Fatalf("parse %q: %v", value, err)
	}

	return day
}

func TestMonday(t *testing.T) {
	cases := []struct {
		name string
		day  string
		want string
	}{
		{"monday stays", "2026-10-05", "2026-10-05"},
		{"tuesday goes back one day", "2026-10-06", "2026-10-05"},
		{"sunday belongs to the week that started six days earlier", "2026-10-11", "2026-10-05"},
		{"week across a month boundary", "2026-10-01", "2026-09-28"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := monday(date(t, tc.day)).Format(domain.DateLayout)
			if got != tc.want {
				t.Errorf("monday(%s) = %s, want %s", tc.day, got, tc.want)
			}
		})
	}
}

func TestDayText(t *testing.T) {
	lessons := []domain.Lesson{
		{
			Date: "2026-10-06", Number: 1, Start: "09:00", End: "10:20",
			Subject: "Теорія рядів", Kind: "Пз", Room: "468",
			Groups: "КН-25-1", Teacher: "Безклубенко Ірина Сергіївна",
		},
		{
			Date: "2026-10-06", Number: 2, Start: "10:30", End: "11:50",
			Subject: "C++ & <templates>", Kind: "Лб", Room: "368",
			Groups: "КН-25-1 (підгр. 2)", Teacher: "",
		},
		{
			Date: "2026-10-07", Number: 1, Start: "09:00", End: "10:20",
			Subject: "Фізичне виховання", Kind: "Пз", Room: "спорт",
			Groups: "КН-25-1", Teacher: "Головко Олена Олександрівна",
		},
	}

	t.Run("a day with classes", func(t *testing.T) {
		text := dayText(testGroup, date(t, "2026-10-06"), lessons)

		for _, want := range []string{
			"Вівторок, 6 жовтня",
			"КН-25-1",
			"<b>1 пара</b> · 09:00-10:20",
			"Теорія рядів <i>(Пз)</i>",
			"📍 468 · Безклубенко Ірина Сергіївна",
			// HTML in a subject would make Telegram reject the message.
			"C++ &amp; &lt;templates&gt;",
			// A class for part of the group says so.
			"👥 КН-25-1 (підгр. 2)",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("day text lacks %q:\n%s", want, text)
			}
		}

		if strings.Contains(text, "Фізичне виховання") {
			t.Errorf("day text shows a class from another day:\n%s", text)
		}
		// The whole group attending is the default and is not repeated.
		if strings.Count(text, "👥") != 1 {
			t.Errorf("day text labels a whole-group class with its group:\n%s", text)
		}
	})

	t.Run("a day without classes", func(t *testing.T) {
		text := dayText(testGroup, date(t, "2026-10-08"), lessons)

		if !strings.Contains(text, textDayEmpty) {
			t.Errorf("empty day lacks %q:\n%s", textDayEmpty, text)
		}
	})
}

func TestWeekText(t *testing.T) {
	weekStart := date(t, "2026-10-05")

	t.Run("days with classes are listed, the rest skipped", func(t *testing.T) {
		lessons := []domain.Lesson{
			{Date: "2026-10-05", Number: 1, Start: "09:00", Subject: "Теорія алгоритмів", Kind: "Лк", Room: "Дистанційно"},
			{Date: "2026-10-07", Number: 3, Start: "12:20", Subject: "Фізичне виховання", Kind: "Пз", Room: "спорт"},
		}

		text := weekText(testGroup, weekStart, lessons)

		for _, want := range []string{
			"Тиждень 05.10-11.10",
			"<b>Понеділок, 05.10</b>",
			"1. 09:00 Теорія алгоритмів <i>(Лк)</i> · Дистанційно",
			"<b>Середа, 07.10</b>",
			"3. 12:20 Фізичне виховання <i>(Пз)</i> · спорт",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("week text lacks %q:\n%s", want, text)
			}
		}

		if strings.Contains(text, "Вівторок") {
			t.Errorf("week text lists a day without classes:\n%s", text)
		}
	})

	t.Run("an empty week", func(t *testing.T) {
		text := weekText(testGroup, weekStart, nil)

		if !strings.Contains(text, textWeekEmpty) {
			t.Errorf("empty week lacks %q:\n%s", textWeekEmpty, text)
		}
	})

	t.Run("a week too long for one message is cut between days", func(t *testing.T) {
		var lessons []domain.Lesson
		for offset := range daysInWeek {
			day := weekStart.AddDate(0, 0, offset).Format(domain.DateLayout)
			for number := 1; number <= 8; number++ {
				lessons = append(lessons, domain.Lesson{
					Date: day, Number: number, Start: "09:00",
					Subject: strings.Repeat("Дуже довга назва дисципліни ", 4), Kind: "Лк", Room: "468",
				})
			}
		}

		text := weekText(testGroup, weekStart, lessons)

		if len([]rune(text)) > weekTextLimit+len([]rune(textWeekTruncated))+2 {
			t.Errorf("week text is %d characters, over the limit", len([]rune(text)))
		}
		if !strings.HasSuffix(text, textWeekTruncated) {
			t.Errorf("a cut week does not say it was cut:\n%s", text[len(text)-200:])
		}
		// Every opened tag is closed: the cut never lands inside a day.
		if strings.Count(text, "<b>") != strings.Count(text, "</b>") {
			t.Errorf("a cut week has unbalanced tags")
		}
	})
}
