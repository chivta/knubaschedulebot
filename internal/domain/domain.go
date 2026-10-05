package domain

// DateLayout is how a Lesson's Date is written. Dates are calendar days in the
// university's timezone, so they travel as strings rather than time.Time: a
// string cannot shift to the neighbouring day when it crosses a timezone.
const DateLayout = "2006-01-02"

// Faculty is one entry of the faculty list on the schedule site.
type Faculty struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Group is an academic group. FacultyID and Course travel with it because the
// schedule site refuses to return a timetable for a group ID alone.
type Group struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	FacultyID int    `json:"faculty_id"`
	Course    int    `json:"course"`
}

// Lesson is one class on one date.
type Lesson struct {
	// Date is the calendar day in DateLayout.
	Date string `json:"date"`
	// Number is the slot in the day's grid, starting at 1.
	Number int `json:"number"`
	// Start and End are wall-clock times such as "09:00".
	Start string `json:"start"`
	End   string `json:"end"`
	// Subject is the full course name.
	Subject string `json:"subject"`
	// Kind is the site's abbreviation for the class type: Лк, Пз, Лб.
	Kind string `json:"kind"`
	// Room is the classroom without the "ауд." prefix.
	Room string `json:"room"`
	// Groups names who attends, as the site prints it. It differs from the
	// group's own name when the class is for a subgroup or a merged stream.
	Groups  string `json:"groups"`
	Teacher string `json:"teacher"`
}
