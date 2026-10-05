package offline

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"time"
)

// What the fake schedule site serves. One faculty, one course, one group, and
// one class on every day that is asked for, so the tests do not depend on the
// calendar date they run on.
const (
	siteFacultyID   = 6
	siteFacultyName = "Факультет тестування (ФТ)"
	siteCourse      = 2
	siteGroupID     = 2323
	siteGroupName   = "ТЕСТ-25"
	siteSubject     = "Основи перевірки ботів"
	siteToken       = "offline-csrf-token"
	siteDateLayout  = "02.01.2006"
)

// fakeSite stands in for mkr.knuba.edu.ua. It reproduces the markup the parser
// reads: the filter form with its three selects, and the timetable.
type fakeSite struct {
	server *httptest.Server
	// timetables counts timetable requests, which is how the tests see
	// whether the cache spared the site a second one.
	timetables atomic.Int64
	// down makes every request fail, as when the site is unreachable.
	down atomic.Bool
}

func newFakeSite() *fakeSite {
	site := &fakeSite{}
	site.server = httptest.NewServer(http.HandlerFunc(site.handle))

	return site
}

func (s *fakeSite) handle(w http.ResponseWriter, r *http.Request) {
	if s.down.Load() {
		http.Error(w, "down", http.StatusInternalServerError)
		return
	}

	w.Header().Set("content-type", "text/html; charset=UTF-8")

	if r.Method == http.MethodGet {
		fmt.Fprint(w, s.page("", "", ""))
		return
	}

	err := r.ParseForm()
	if err != nil || r.PostForm.Get("_csrf-frontend") != siteToken {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	faculty := r.PostForm.Get("TimeTableForm[facultyId]")
	course := r.PostForm.Get("TimeTableForm[course]")
	group := r.PostForm.Get("TimeTableForm[groupId]")

	switch {
	case group != "":
		s.timetables.Add(1)
		table := timetable(r.PostForm.Get("TimeTableForm[dateStart]"), r.PostForm.Get("TimeTableForm[dateEnd]"))
		fmt.Fprint(w, s.page(courseOptions(), groupOptions(), table))
	case course != "":
		fmt.Fprint(w, s.page(courseOptions(), groupOptions(), ""))
	case faculty != "":
		fmt.Fprint(w, s.page(courseOptions(), "", ""))
	default:
		fmt.Fprint(w, s.page("", "", ""))
	}
}

// page renders the filter form around whatever the request unlocked.
func (s *fakeSite) page(courses, groups, table string) string {
	return fmt.Sprintf(`<!DOCTYPE html><html><body>
<form id="filter-form" method="POST">
<input type="hidden" name="_csrf-frontend" value="%s">
<select id="timetableform-facultyid" name="TimeTableForm[facultyId]">
<option value="">--Факультет--</option>
<option value="%d">%s</option>
</select>
<select id="timetableform-course" name="TimeTableForm[course]">%s</select>
<select id="timetableform-groupid" name="TimeTableForm[groupId]">%s</select>
</form>%s</body></html>`, siteToken, siteFacultyID, siteFacultyName, courses, groups, table)
}

func courseOptions() string {
	return fmt.Sprintf(`<option value="">--Курс--</option><option value="%d">%d</option>`, siteCourse, siteCourse)
}

func groupOptions() string {
	return fmt.Sprintf(`<option value="">--Група--</option><option value="%d">%s</option>`, siteGroupID, siteGroupName)
}

// timetable renders one class in slot 1 on every day of the requested range.
func timetable(from, to string) string {
	start, err := time.Parse(siteDateLayout, from)
	if err != nil {
		return ""
	}
	end, err := time.Parse(siteDateLayout, to)
	if err != nil {
		return ""
	}

	var cells strings.Builder
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		fmt.Fprintf(&cells, `<td><div class="cell"><div class="lesson-1 ">
<div data-toggle="popover" title="%s 1 пара" data-content="%s[Лк]<br>ауд. 101<br>%s<br>Тестовий Викладач<br>Додано:  01.09.2026">x</div>
</div></div></td>`, day.Format(siteDateLayout), siteSubject, siteGroupName)
	}

	return `<table class="table" id="timeTable"><tr>
<th class="headcol"><span class="lesson">1 пара</span><span class="start">09:00</span><span class="end">10:20</span></th>` +
		cells.String() + `</tr></table>`
}
