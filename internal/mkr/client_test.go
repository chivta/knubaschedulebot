package mkr

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/chivta/knubaschedulebot/internal/domain"
)

const (
	testToken  = "ph1KGY5PNVVWKk1_a9LQqIFsiszPZPl9aXtxZ7TUOEGSbAdp4DtDEAlBPxYTpODc5xjiqf8uvw4zIzAi7IwAMQ=="
	testCookie = "advanced-frontend"
)

type recorded struct {
	method string
	cookie bool
	ua     string
	form   map[string][]string
}

// fakeSite serves the form on GET and, on POST, the fixture picked by pick.
// It rejects POSTs without the cookie or token with 400, as the real site does.
func fakeSite(t *testing.T, pick func(form map[string][]string) string) (*httptest.Server, func() []recorded) {
	t.Helper()
	var mu sync.Mutex
	var reqs []recorded
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := recorded{method: r.Method, ua: r.UserAgent()}
		_, cerr := r.Cookie(testCookie)
		rec.cookie = cerr == nil
		if r.Method == http.MethodGet {
			http.SetCookie(w, &http.Cookie{Name: testCookie, Value: "sess", Path: "/"})
			mu.Lock()
			reqs = append(reqs, rec)
			mu.Unlock()
			serveFile(t, w, "faculties.html")
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		rec.form = r.PostForm
		mu.Lock()
		reqs = append(reqs, rec)
		mu.Unlock()
		if !rec.cookie || r.PostFormValue(csrfField) != testToken {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		serveFile(t, w, pick(r.PostForm))
	}))
	t.Cleanup(srv.Close)
	return srv, func() []recorded {
		mu.Lock()
		defer mu.Unlock()
		return append([]recorded(nil), reqs...)
	}
}

func serveFile(t *testing.T, w http.ResponseWriter, name string) {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Errorf("read fixture: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(b)
}

func assertField(t *testing.T, form map[string][]string, key, want string) {
	t.Helper()
	got, ok := form[key]
	if !ok || len(got) != 1 || got[0] != want {
		t.Fatalf("form[%s] = %v, want %q", key, got, want)
	}
}

func TestClientFaculties(t *testing.T) {
	srv, reqs := fakeSite(t, nil)
	got, err := New(srv.URL, time.Second).Faculties(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 11 {
		t.Fatalf("len = %d, want 11", len(got))
	}
	r := reqs()
	if len(r) != 1 || r[0].method != http.MethodGet {
		t.Fatalf("requests = %+v, want a single GET", r)
	}
	if r[0].ua != userAgent {
		t.Fatalf("user agent = %q", r[0].ua)
	}
}

func TestClientCourses(t *testing.T) {
	srv, reqs := fakeSite(t, func(map[string][]string) string { return "courses.html" })
	got, err := New(srv.URL, time.Second).Courses(context.Background(), 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Fatalf("courses = %v", got)
	}
	r := reqs()
	if len(r) != 2 || r[0].method != http.MethodGet || r[1].method != http.MethodPost || !r[1].cookie {
		t.Fatalf("requests = %+v", r)
	}
	assertField(t, r[1].form, fieldFaculty, "6")
	assertField(t, r[1].form, fieldCourse, "")
	assertField(t, r[1].form, fieldGroup, "")
	assertField(t, r[1].form, fieldStructure, "0")
}

func TestClientGroups(t *testing.T) {
	srv, reqs := fakeSite(t, func(map[string][]string) string { return "groups.html" })
	got, err := New(srv.URL, time.Second).Groups(context.Background(), 6, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 44 {
		t.Fatalf("len = %d", len(got))
	}
	for _, g := range got {
		if g.FacultyID != 6 || g.Course != 2 {
			t.Fatalf("group not filled: %+v", g)
		}
	}
	assertField(t, reqs()[1].form, fieldCourse, "2")
}

func TestClientLessons(t *testing.T) {
	srv, reqs := fakeSite(t, func(map[string][]string) string { return "week.html" })
	group := domain.Group{ID: 2323, Name: "КН-25-1", FacultyID: 6, Course: 2}
	got, err := New(srv.URL, time.Second).Lessons(context.Background(), group, "2026-10-05", "2026-10-11")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 15 {
		t.Fatalf("len = %d, want 15", len(got))
	}
	for i := 1; i < len(got); i++ {
		a, b := got[i-1], got[i]
		if a.Date > b.Date || (a.Date == b.Date && a.Number > b.Number) {
			t.Fatalf("unsorted: %+v before %+v", a, b)
		}
	}

	r := reqs()
	if len(r) != 2 || r[0].method != http.MethodGet || r[1].method != http.MethodPost {
		t.Fatalf("requests = %+v, want GET then POST", r)
	}
	if !r[1].cookie {
		t.Fatal("POST lacks the session cookie")
	}
	form := r[1].form
	assertField(t, form, csrfField, testToken)
	assertField(t, form, fieldStructure, "0")
	assertField(t, form, fieldFaculty, "6")
	assertField(t, form, fieldCourse, "2")
	assertField(t, form, fieldGroup, "2323")
	assertField(t, form, fieldDateStart, "05.10.2026")
	assertField(t, form, fieldDateEnd, "11.10.2026")
}

func TestClientLessonsEmptyWeek(t *testing.T) {
	srv, _ := fakeSite(t, func(map[string][]string) string { return "empty_week.html" })
	got, err := New(srv.URL, time.Second).Lessons(context.Background(), domain.Group{ID: 2323, FacultyID: 6, Course: 2}, "2027-01-11", "2027-01-17")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("got %#v, want empty non-nil slice", got)
	}
}

func TestClientLessonsBadDate(t *testing.T) {
	_, err := New("http://127.0.0.1:0", time.Second).Lessons(context.Background(), domain.Group{}, "05.10.2026", "2026-10-11")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestClientUpstreamErrors(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"GET 500", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) }},
		{"no token", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("<html></html>")) }},
		{"POST 500", func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			serveFile(t, w, "faculties.html")
		}},
		{"POST without timetable", func(w http.ResponseWriter, _ *http.Request) { serveFile(t, w, "faculties.html") }},
	}
	group := domain.Group{ID: 2323, FacultyID: 6, Course: 2}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()
			_, err := New(srv.URL, time.Second).Lessons(context.Background(), group, "2026-10-05", "2026-10-11")
			if !errors.Is(err, domain.ErrUpstream) {
				t.Fatalf("err = %v, want ErrUpstream", err)
			}
		})
	}
}

func TestClientNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	_, err := New(url, time.Second).Faculties(context.Background())
	if !errors.Is(err, domain.ErrUpstream) {
		t.Fatalf("err = %v, want ErrUpstream", err)
	}
}
