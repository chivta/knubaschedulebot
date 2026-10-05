package mkr

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/chivta/knubaschedulebot/internal/domain"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return string(b)
}

func TestParseToken(t *testing.T) {
	tests := []struct {
		name    string
		html    string
		want    string
		wantErr bool
	}{
		{"form page", fixture(t, "faculties.html"), "ph1KGY5PNVVWKk1_a9LQqIFsiszPZPl9aXtxZ7TUOEGSbAdp4DtDEAlBPxYTpODc5xjiqf8uvw4zIzAi7IwAMQ==", false},
		{"no token", "<html><body></body></html>", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseToken(strings.NewReader(tt.html))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, domain.ErrUpstream) {
				t.Fatalf("err = %v, want ErrUpstream", err)
			}
			if got != tt.want {
				t.Fatalf("token = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseFaculties(t *testing.T) {
	got, err := parseFaculties(strings.NewReader(fixture(t, "faculties.html")))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 11 {
		t.Fatalf("len = %d, want 11", len(got))
	}
	want := domain.Faculty{ID: 6, Name: "Факультет автоматизації і інформаційних технологій (ФАІТ)"}
	found := false
	for _, f := range got {
		if f == want {
			found = true
		}
		if f.ID == 0 {
			t.Fatalf("placeholder leaked: %+v", f)
		}
	}
	if !found {
		t.Fatalf("%+v not in %+v", want, got)
	}
}

func TestParseCourses(t *testing.T) {
	got, err := parseCourses(strings.NewReader(fixture(t, "courses.html")))
	if err != nil {
		t.Fatal(err)
	}
	want := []int{1, 2, 3, 4, 5}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestParseGroups(t *testing.T) {
	got, err := parseGroups(strings.NewReader(fixture(t, "groups.html")))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 44 {
		t.Fatalf("len = %d, want 44", len(got))
	}
	found := false
	for _, g := range got {
		if g.Name != strings.TrimSpace(g.Name) {
			t.Fatalf("untrimmed name %q", g.Name)
		}
		if g.ID == 2323 && g.Name == "КН-25-1" {
			found = true
		}
	}
	if !found {
		t.Fatal("group 2323 КН-25-1 not found")
	}
}

func TestParseMissingSelect(t *testing.T) {
	page := fixture(t, "faculties.html") // has no groups
	_, err := parseGroups(strings.NewReader("<html></html>"))
	if !errors.Is(err, domain.ErrUpstream) {
		t.Fatalf("err = %v, want ErrUpstream", err)
	}
	_, err = parseCourses(strings.NewReader("<html></html>"))
	if !errors.Is(err, domain.ErrUpstream) {
		t.Fatalf("err = %v, want ErrUpstream", err)
	}
	_, err = parseLessons(strings.NewReader(page))
	if !errors.Is(err, domain.ErrUpstream) {
		t.Fatalf("no timetable: err = %v, want ErrUpstream", err)
	}
}

func TestParseLessons(t *testing.T) {
	got, err := parseLessons(strings.NewReader(fixture(t, "week.html")))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 15 {
		t.Fatalf("len = %d, want 15", len(got))
	}

	first := got[0]
	wantFirst := domain.Lesson{
		Date: "2026-10-05", Number: 1, Start: "09:00", End: "10:20",
		Kind: "Лк", Room: "Дистанційно", Groups: "КН-25-1", Teacher: "Серпінська Ольга Ігорівна",
	}
	wantFirst.Subject = first.Subject // site data mixes Latin i; checked separately below
	if first != wantFirst {
		t.Fatalf("first = %+v, want %+v", first, wantFirst)
	}
	if !strings.HasPrefix(first.Subject, "Те") || !strings.HasSuffix(first.Subject, "алгоритмiв") {
		t.Fatalf("subject = %q", first.Subject)
	}

	days := map[string]int{}
	for _, l := range got {
		days[l.Date]++
		if l.Start == "" || l.End == "" || l.Subject == "" || l.Kind == "" {
			t.Fatalf("incomplete lesson %+v", l)
		}
		if strings.HasSuffix(l.Subject, " ") || strings.Contains(l.Subject, "[") {
			t.Fatalf("subject not clean: %q", l.Subject)
		}
		if strings.HasPrefix(l.Room, roomPrefix) {
			t.Fatalf("room keeps prefix: %q", l.Room)
		}
	}
	for _, d := range []string{"2026-10-05", "2026-10-06", "2026-10-07", "2026-10-09"} {
		if days[d] == 0 {
			t.Fatalf("no lessons on %s: %v", d, days)
		}
	}
	if len(days) != 4 {
		t.Fatalf("days = %v, want 4 days", days)
	}

	// Entity-escaped apostrophe is unescaped; the trailing space before [Лк] is trimmed.
	var apostrophe bool
	for _, l := range got {
		if strings.Contains(l.Subject, "'") {
			apostrophe = true
		}
	}
	if !apostrophe {
		t.Fatal("expected a subject with an unescaped apostrophe")
	}
}

func TestParseLessonsEmpty(t *testing.T) {
	got, err := parseLessons(strings.NewReader(fixture(t, "empty_week.html")))
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("got %#v, want empty non-nil slice", got)
	}
}

func TestParseLessonsShapes(t *testing.T) {
	const head = `<table id="timeTable"><tr><th class="headcol"><span class="lesson">2 пара</span><span class="start">10:30</span><span class="end">11:50</span></th><td>`
	const tail = `</td></tr></table>`
	tests := []struct {
		name string
		body string
		want []domain.Lesson
	}{
		{
			name: "subgroups in one cell",
			body: `<div data-toggle="popover" title="05.10.2026 2 пара" data-content="Англ [Пз]<br>ауд. 101<br>КН-25-1<br>Іваненко І.І.<br>Додано:  01.09.2026"></div>` +
				`<div data-toggle="popover" title="05.10.2026 2 пара" data-content="Нім [Пз]<br>ауд. 102<br>КН-25-1<br>Петренко П.П.<br>Додано:  01.09.2026"></div>`,
			want: []domain.Lesson{
				{Date: "2026-10-05", Number: 2, Start: "10:30", End: "11:50", Subject: "Англ", Kind: "Пз", Room: "101", Groups: "КН-25-1", Teacher: "Іваненко І.І."},
				{Date: "2026-10-05", Number: 2, Start: "10:30", End: "11:50", Subject: "Нім", Kind: "Пз", Room: "102", Groups: "КН-25-1", Teacher: "Петренко П.П."},
			},
		},
		{
			name: "missing teacher and room",
			body: `<div data-toggle="popover" title="05.10.2026 2 пара" data-content="Фізика[Лк]<br>КН-25-1<br>Додано:  01.09.2026"></div>`,
			want: []domain.Lesson{
				{Date: "2026-10-05", Number: 2, Start: "10:30", End: "11:50", Subject: "Фізика", Kind: "Лк", Groups: "КН-25-1"},
			},
		},
		{
			name: "subject only",
			body: `<div data-toggle="popover" title="05.10.2026 2 пара" data-content="Фізика[Лк]"></div>`,
			want: []domain.Lesson{
				{Date: "2026-10-05", Number: 2, Start: "10:30", End: "11:50", Subject: "Фізика", Kind: "Лк"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLessons(strings.NewReader(head + tt.body + tail))
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("lesson %d = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseLessonsIgnoresScriptMention(t *testing.T) {
	page := `<script>$('[data-toggle="popover"]').popover();</script><table id="timeTable"></table>`
	got, err := parseLessons(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

// block_week.html (live page) has classes spanning several slots: the site
// prefixes their data-content with a time range and repeats the popover per slot.
func TestParseLessonsMultiSlotBlock(t *testing.T) {
	got, err := parseLessons(strings.NewReader(fixture(t, "block_week.html")))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 18 {
		t.Fatalf("len = %d, want 18", len(got))
	}
	for _, l := range got {
		if strings.Contains(l.Subject, ":") || l.Kind == "" || l.Teacher == "" || l.Groups == "" {
			t.Fatalf("misparsed lesson %+v", l)
		}
	}
	first := got[0]
	if first.Date != "2026-10-05" || first.Number != 3 || first.Start != "12:20" || first.Kind != "Пз" ||
		first.Room != "106-А" || first.Groups != "АРХс-25" ||
		first.Teacher != "Третяк Максим Едуардович, Желтовський Володимир Васильович, Гуменюк Ганна Володимирівна" {
		t.Fatalf("first = %+v", first)
	}
	if got[1].Number != 4 || got[1].Subject != first.Subject {
		t.Fatalf("second = %+v", got[1])
	}
}
