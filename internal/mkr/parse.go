package mkr

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/chivta/knubaschedulebot/internal/domain"
)

const (
	selectFaculty = "#timetableform-facultyid"
	selectCourse  = "#timetableform-course"
	selectGroup   = "#timetableform-groupid"

	selectCSRF    = `input[name="` + csrfField + `"]`
	selectTable   = "#timeTable"
	selectPopover = `#timeTable div[data-toggle="popover"]`
	selectHeadcol = "#timeTable th.headcol"

	// siteDateLayout is how the site writes dates in titles and forms.
	siteDateLayout = "02.01.2006"

	roomPrefix  = "ауд."
	addedPrefix = "Додано"

	// titleParts is the field count of a popover title: date, slot number, "пара".
	titleParts = 3
)

var (
	brTag = regexp.MustCompile(`(?i)<br\s*/?>`)
	// timeRangeLine is the extra first line the site adds to classes that span
	// several slots, e.g. "12:20-15:10".
	timeRangeLine = regexp.MustCompile(`^\d{1,2}:\d{2}-\d{1,2}:\d{2}$`)
)

func newDoc(r io.Reader) (*goquery.Document, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, fmt.Errorf("%w: parse html: %v", domain.ErrUpstream, err)
	}
	return doc, nil
}

// parseToken returns the CSRF token from the filter form.
func parseToken(r io.Reader) (string, error) {
	doc, err := newDoc(r)
	if err != nil {
		return "", err
	}
	token, ok := doc.Find(selectCSRF).First().Attr("value")
	if !ok || token == "" {
		return "", fmt.Errorf("%w: csrf token not found", domain.ErrUpstream)
	}
	return token, nil
}

type option struct {
	value string
	text  string
}

// selectOptions returns the options of the select matching sel, without the
// placeholder (the option with an empty value).
func selectOptions(doc *goquery.Document, sel string) ([]option, error) {
	s := doc.Find(sel)
	if s.Length() == 0 {
		return nil, fmt.Errorf("%w: select %s not found", domain.ErrUpstream, sel)
	}
	var opts []option
	s.Find("option").Each(func(_ int, o *goquery.Selection) {
		value := strings.TrimSpace(o.AttrOr("value", ""))
		if value == "" {
			return
		}
		opts = append(opts, option{value: value, text: strings.TrimSpace(o.Text())})
	})
	return opts, nil
}

// parseFaculties reads the faculty select of the filter form.
func parseFaculties(r io.Reader) ([]domain.Faculty, error) {
	doc, err := newDoc(r)
	if err != nil {
		return nil, err
	}
	opts, err := selectOptions(doc, selectFaculty)
	if err != nil {
		return nil, err
	}
	faculties := make([]domain.Faculty, 0, len(opts))
	for _, o := range opts {
		id, err := strconv.Atoi(o.value)
		if err != nil {
			return nil, fmt.Errorf("%w: faculty id %q: %v", domain.ErrUpstream, o.value, err)
		}
		faculties = append(faculties, domain.Faculty{ID: id, Name: o.text})
	}
	return faculties, nil
}

// parseCourses reads the course select of a page posted with a faculty.
func parseCourses(r io.Reader) ([]int, error) {
	doc, err := newDoc(r)
	if err != nil {
		return nil, err
	}
	opts, err := selectOptions(doc, selectCourse)
	if err != nil {
		return nil, err
	}
	courses := make([]int, 0, len(opts))
	for _, o := range opts {
		course, err := strconv.Atoi(o.value)
		if err != nil {
			return nil, fmt.Errorf("%w: course %q: %v", domain.ErrUpstream, o.value, err)
		}
		courses = append(courses, course)
	}
	return courses, nil
}

// parseGroups reads the group select of a page posted with faculty and course.
// FacultyID and Course are left for the caller to fill.
func parseGroups(r io.Reader) ([]domain.Group, error) {
	doc, err := newDoc(r)
	if err != nil {
		return nil, err
	}
	opts, err := selectOptions(doc, selectGroup)
	if err != nil {
		return nil, err
	}
	groups := make([]domain.Group, 0, len(opts))
	for _, o := range opts {
		id, err := strconv.Atoi(o.value)
		if err != nil {
			return nil, fmt.Errorf("%w: group id %q: %v", domain.ErrUpstream, o.value, err)
		}
		groups = append(groups, domain.Group{ID: id, Name: o.text})
	}
	return groups, nil
}

type slot struct {
	start string
	end   string
}

// parseSlots maps slot number to its times, from the row headers of the table.
// The headers repeat for every day, the first occurrence wins.
func parseSlots(doc *goquery.Document) (map[int]slot, error) {
	slots := map[int]slot{}
	var perr error
	doc.Find(selectHeadcol).Each(func(_ int, th *goquery.Selection) {
		if perr != nil {
			return
		}
		label := strings.Fields(th.Find("span.lesson").Text())
		if len(label) == 0 {
			perr = fmt.Errorf("%w: slot header without number", domain.ErrUpstream)
			return
		}
		n, err := strconv.Atoi(label[0])
		if err != nil {
			perr = fmt.Errorf("%w: slot number %q: %v", domain.ErrUpstream, label[0], err)
			return
		}
		_, seen := slots[n]
		if seen {
			return
		}
		slots[n] = slot{
			start: strings.TrimSpace(th.Find("span.start").Text()),
			end:   strings.TrimSpace(th.Find("span.end").Text()),
		}
	})
	return slots, perr
}

// parseLessons reads every class in the timetable. A table with no classes
// gives an empty slice; a page without the table is an upstream error.
func parseLessons(r io.Reader) ([]domain.Lesson, error) {
	doc, err := newDoc(r)
	if err != nil {
		return nil, err
	}
	if doc.Find(selectTable).Length() == 0 {
		return nil, fmt.Errorf("%w: timetable not found", domain.ErrUpstream)
	}
	slots, err := parseSlots(doc)
	if err != nil {
		return nil, err
	}

	lessons := []domain.Lesson{}
	var perr error
	doc.Find(selectPopover).Each(func(_ int, div *goquery.Selection) {
		if perr != nil {
			return
		}
		lesson, err := parseLesson(div, slots)
		if err != nil {
			perr = err
			return
		}
		lessons = append(lessons, lesson)
	})
	if perr != nil {
		return nil, perr
	}
	return lessons, nil
}

// parseLesson builds one Lesson from a popover div: date and slot number come
// from the title, the rest from the <br>-separated data-content.
func parseLesson(div *goquery.Selection, slots map[int]slot) (domain.Lesson, error) {
	title := div.AttrOr("title", "")
	parts := strings.Fields(title)
	if len(parts) != titleParts {
		return domain.Lesson{}, fmt.Errorf("%w: lesson title %q", domain.ErrUpstream, title)
	}
	date, err := time.Parse(siteDateLayout, parts[0])
	if err != nil {
		return domain.Lesson{}, fmt.Errorf("%w: lesson date %q: %v", domain.ErrUpstream, parts[0], err)
	}
	number, err := strconv.Atoi(parts[1])
	if err != nil {
		return domain.Lesson{}, fmt.Errorf("%w: lesson number %q: %v", domain.ErrUpstream, parts[1], err)
	}

	lesson := domain.Lesson{
		Date:   date.Format(domain.DateLayout),
		Number: number,
		Start:  slots[number].start,
		End:    slots[number].end,
	}

	lines := brTag.Split(div.AttrOr("data-content", ""), -1)
	if len(lines) > 0 && timeRangeLine.MatchString(strings.TrimSpace(lines[0])) {
		lines = lines[1:]
	}

	var rest []string
	for i, line := range lines {
		// The site's data has stray double spaces inside names.
		line = strings.Join(strings.Fields(line), " ")
		switch {
		case i == 0:
			lesson.Subject, lesson.Kind = splitSubject(line)
		case strings.HasPrefix(line, roomPrefix):
			lesson.Room = strings.TrimSpace(strings.TrimPrefix(line, roomPrefix))
		case strings.HasPrefix(line, addedPrefix):
		case line != "":
			rest = append(rest, line)
		}
	}
	if len(rest) > 0 {
		lesson.Groups = rest[0]
	}
	if len(rest) > 1 {
		lesson.Teacher = rest[1]
	}
	return lesson, nil
}

// splitSubject splits "Subject [Kind]" into its two parts.
func splitSubject(line string) (subject, kind string) {
	open := strings.LastIndex(line, "[")
	closing := strings.LastIndex(line, "]")
	if open < 0 || closing < open {
		return line, ""
	}
	return strings.TrimSpace(line[:open]), strings.TrimSpace(line[open+1 : closing])
}
