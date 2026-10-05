package bot

import (
	"fmt"

	"github.com/rs/zerolog/log"
	tele "gopkg.in/telebot.v4"

	"github.com/chivta/knubaschedulebot/internal/domain"
)

// The group picker walks the same three steps as the form on the schedule
// site: faculty, course, group. Each step rewrites the picker message in place.

// handleChooseGroup opens the picker, from the menu or from its "back" button.
func (b *Bot) handleChooseGroup(c tele.Context) error {
	return b.showFaculties(c, textChooseFaculty)
}

// showFaculties shows the first step of the picker under the given prompt.
func (b *Bot) showFaculties(c tele.Context, prompt string) error {
	faculties, err := b.schedule.Faculties(b.ctx)
	if err != nil {
		return b.fail(c, err)
	}

	return b.render(c, prompt, facultyKeyboard(faculties))
}

// handleFaculty shows the courses of the tapped faculty.
func (b *Bot) handleFaculty(c tele.Context) error {
	args, err := intArgs(c, 1)
	if err != nil {
		return b.fail(c, err)
	}
	facultyID := args[0]

	courses, err := b.schedule.Courses(b.ctx, facultyID)
	if err != nil {
		return b.fail(c, err)
	}

	return b.render(c, textChooseCourse, courseKeyboard(facultyID, courses))
}

// handleCourse shows the groups of the tapped course.
func (b *Bot) handleCourse(c tele.Context) error {
	args, err := intArgs(c, 2)
	if err != nil {
		return b.fail(c, err)
	}
	facultyID, course := args[0], args[1]

	groups, err := b.schedule.Groups(b.ctx, facultyID, course)
	if err != nil {
		return b.fail(c, err)
	}

	return b.render(c, textChooseGroup, groupKeyboard(facultyID, course, groups))
}

// handleGroup saves the tapped group for the sender and hands over the menu.
func (b *Bot) handleGroup(c tele.Context) error {
	args, err := intArgs(c, 3)
	if err != nil {
		return b.fail(c, err)
	}
	facultyID, course, groupID := args[0], args[1], args[2]

	// The button only carries IDs. Listing the course again, from cache in
	// practice, recovers the name and proves the group really exists there.
	groups, err := b.schedule.Groups(b.ctx, facultyID, course)
	if err != nil {
		return b.fail(c, err)
	}

	group, err := findGroup(groups, groupID)
	if err != nil {
		return b.fail(c, err)
	}

	err = b.users.SetGroup(b.ctx, c.Sender().ID, group)
	if err != nil {
		return b.fail(c, err)
	}

	log.Info().Int64("user_id", c.Sender().ID).Int("group_id", group.ID).Msg("user picked a group")

	b.removePicker(c)

	return b.send(c, fmt.Sprintf(textGroupSaved, escape(group.Name)), mainMenu())
}

// removePicker deletes the picker message once it has done its job, so the
// chat keeps one confirmation rather than a dead keyboard above it.
func (b *Bot) removePicker(c tele.Context) {
	err := b.sends.wait(b.ctx)
	if err != nil {
		return
	}

	err = c.Delete()
	if err != nil {
		log.Debug().Err(err).Msg("failed to delete the group picker")
	}
}

// findGroup picks a group out of a course listing by ID.
func findGroup(groups []domain.Group, id int) (domain.Group, error) {
	for _, group := range groups {
		if group.ID == id {
			return group, nil
		}
	}

	return domain.Group{}, domain.ErrNotFound
}
