// Package mkr is a client and HTML parser for the university schedule site
// (ASU MKR, a Yii2 app without a JSON API).
package mkr

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chivta/knubaschedulebot/internal/domain"
)

const (
	// userAgent is sent on every request; the site is meant for browsers.
	userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"

	groupPath = "/time-table/group?type=0"

	csrfField       = "_csrf-frontend"
	fieldStructure  = "TimeTableForm[structureId]"
	fieldFaculty    = "TimeTableForm[facultyId]"
	fieldCourse     = "TimeTableForm[course]"
	fieldGroup      = "TimeTableForm[groupId]"
	fieldDateStart  = "TimeTableForm[dateStart]"
	fieldDateEnd    = "TimeTableForm[dateEnd]"
	structureAll    = "0"
	contentTypeForm = "application/x-www-form-urlencoded"
)

// Client talks to the schedule site. It holds no session state: every call
// opens its own cookie session, so it is safe for concurrent use.
type Client struct {
	baseURL string
	timeout time.Duration
}

// New builds a client. baseURL has no trailing slash, e.g. "https://mkr.knuba.edu.ua".
func New(baseURL string, timeout time.Duration) *Client {
	return &Client{baseURL: baseURL, timeout: timeout}
}

// session is one cookie jar plus the CSRF token issued with it.
type session struct {
	http  *http.Client
	url   string
	token string
}

// open performs the initial GET and returns the session with the raw page.
func (c *Client) open(ctx context.Context) (*session, []byte, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: cookie jar: %v", domain.ErrUpstream, err)
	}
	s := &session{
		http: &http.Client{Timeout: c.timeout, Jar: jar},
		url:  c.baseURL + groupPath,
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: GET form: %v", domain.ErrUpstream, err)
	}
	body, err := s.do(req, "GET form")
	if err != nil {
		return nil, nil, err
	}
	s.token, err = parseToken(bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	return s, body, nil
}

// post sends the filter form with the session token and returns the page.
func (s *session) post(ctx context.Context, what string, fields url.Values) ([]byte, error) {
	fields.Set(csrfField, s.token)
	fields.Set(fieldStructure, structureAll)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, strings.NewReader(fields.Encode()))
	if err != nil {
		return nil, fmt.Errorf("%w: POST %s: %v", domain.ErrUpstream, what, err)
	}
	req.Header.Set("Content-Type", contentTypeForm)
	return s.do(req, "POST "+what)
}

func (s *session) do(req *http.Request, what string) ([]byte, error) {
	req.Header.Set("User-Agent", userAgent)
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", domain.ErrUpstream, what, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %s: status %d", domain.ErrUpstream, what, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: read body: %v", domain.ErrUpstream, what, err)
	}
	return body, nil
}

// Faculties lists all faculties.
func (c *Client) Faculties(ctx context.Context) ([]domain.Faculty, error) {
	_, body, err := c.open(ctx)
	if err != nil {
		return nil, err
	}
	return parseFaculties(bytes.NewReader(body))
}

// Courses lists the course numbers of a faculty.
func (c *Client) Courses(ctx context.Context, facultyID int) ([]int, error) {
	s, _, err := c.open(ctx)
	if err != nil {
		return nil, err
	}
	fields := url.Values{}
	fields.Set(fieldFaculty, strconv.Itoa(facultyID))
	fields.Set(fieldCourse, "")
	fields.Set(fieldGroup, "")
	body, err := s.post(ctx, "courses", fields)
	if err != nil {
		return nil, err
	}
	return parseCourses(bytes.NewReader(body))
}

// Groups lists the groups of a course. FacultyID and Course are filled on every group.
func (c *Client) Groups(ctx context.Context, facultyID, course int) ([]domain.Group, error) {
	s, _, err := c.open(ctx)
	if err != nil {
		return nil, err
	}
	fields := url.Values{}
	fields.Set(fieldFaculty, strconv.Itoa(facultyID))
	fields.Set(fieldCourse, strconv.Itoa(course))
	fields.Set(fieldGroup, "")
	body, err := s.post(ctx, "groups", fields)
	if err != nil {
		return nil, err
	}
	groups, err := parseGroups(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for i := range groups {
		groups[i].FacultyID = facultyID
		groups[i].Course = course
	}
	return groups, nil
}

// Lessons returns classes between from and to inclusive, both in domain.DateLayout,
// sorted by date then lesson number. No classes is an empty slice and a nil error.
func (c *Client) Lessons(ctx context.Context, group domain.Group, from, to string) ([]domain.Lesson, error) {
	start, err := siteDate(from)
	if err != nil {
		return nil, err
	}
	end, err := siteDate(to)
	if err != nil {
		return nil, err
	}
	s, _, err := c.open(ctx)
	if err != nil {
		return nil, err
	}
	fields := url.Values{}
	fields.Set(fieldFaculty, strconv.Itoa(group.FacultyID))
	fields.Set(fieldCourse, strconv.Itoa(group.Course))
	fields.Set(fieldGroup, strconv.Itoa(group.ID))
	fields.Set(fieldDateStart, start)
	fields.Set(fieldDateEnd, end)
	body, err := s.post(ctx, "timetable", fields)
	if err != nil {
		return nil, err
	}
	lessons, err := parseLessons(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	sort.SliceStable(lessons, func(i, j int) bool {
		if lessons[i].Date != lessons[j].Date {
			return lessons[i].Date < lessons[j].Date
		}
		return lessons[i].Number < lessons[j].Number
	})
	return lessons, nil
}

// siteDate converts a domain.DateLayout date to the DD.MM.YYYY the site expects.
func siteDate(date string) (string, error) {
	t, err := time.Parse(domain.DateLayout, date)
	if err != nil {
		return "", fmt.Errorf("%w: date %q: %v", domain.ErrInvalidInput, date, err)
	}
	return t.Format(siteDateLayout), nil
}
