// Package offline drives the bot against a fake Bot API server, a fake schedule
// site and an in-memory Redis. No network, no credentials, no Telegram: it runs
// anywhere, including CI, and finishes in seconds. The bot itself is untouched,
// a real process with a real HTTP client and a real polling loop, so the wiring
// these tests cover is the real wiring.
//
//	go test ./test/offline/
package offline

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	_ "modernc.org/sqlite"

	"github.com/chivta/knubaschedulebot/internal/tgfake"
)

const (
	adminID    = int64(1001)
	userID     = int64(2002)
	strangerID = int64(3003)

	// pickerMessageID is the message the test pretends the inline keyboard is
	// attached to. The fake does not hand out the IDs of sent messages, and the
	// bot only echoes this one back when it edits the picker.
	pickerMessageID = int64(777)

	// replyWait is generous relative to a local round trip, so a slow CI
	// machine does not produce a flake.
	replyWait = 30 * time.Second
	pollTick  = 20 * time.Millisecond

	// supportContact is who the bot must send strangers to.
	supportContact = "@ukbotsup"

	// fakeUsername is the username tgfake gives every user it invents.
	fakeUsername = "tester"
)

// TestStrangerIsSentToSupport is the one that matters most. The bot's username
// is public, so the allow list is all that stands between a stranger and the
// bot, including the command that edits the allow list itself.
func TestStrangerIsSentToSupport(t *testing.T) {
	h := startBot(t)

	h.fake.SendCommand(strangerID, "/start")

	denial := h.waitForMessageTo(strangerID, supportContact)
	if !strings.Contains(denial, fmt.Sprint(strangerID)) {
		t.Errorf("the denial does not tell the stranger their ID: %q", denial)
	}

	t.Run("cannot allow themselves", func(t *testing.T) {
		h.fake.SendCommand(strangerID, fmt.Sprintf("/allow %d", strangerID))

		h.waitForMessages(strangerID, supportContact, 2)
		if h.isAllowed(strangerID) {
			t.Errorf("a stranger added themselves to the allow list")
		}
	})

	t.Run("cannot use a guessed button", func(t *testing.T) {
		// Button payloads are guessable and arrive from anyone.
		h.fake.ClickButton(strangerID, pickerMessageID,
			fmt.Sprintf("\fgroup|%d|%d|%d", siteFacultyID, siteCourse, siteGroupID))

		_, err := h.fake.WaitForCall("answerCallbackQuery", "text", supportContact, replyWait)
		if err != nil {
			t.Fatalf("the tap was not refused: %v", err)
		}
		if group := h.storedGroup(strangerID); group != 0 {
			t.Errorf("a stranger saved group %d through a guessed button", group)
		}
	})
}

func TestAdminManagesAllowList(t *testing.T) {
	h := startBot(t)

	h.fake.SendCommand(adminID, fmt.Sprintf("/allow %d", userID))

	h.waitForMessageTo(adminID, "додано")
	// The reply claiming success and the row being on disk are different
	// claims. Read the bot's own store and check the second one.
	if !h.isAllowed(userID) {
		t.Fatalf("the allowed user is not in the store")
	}
	h.waitForMessageTo(userID, "надано доступ")

	t.Run("the allowed user gets in", func(t *testing.T) {
		h.fake.SendCommand(userID, "/start")

		h.waitForMessageTo(userID, "факультет")
	})

	t.Run("the allowed user is not an admin", func(t *testing.T) {
		h.fake.SendCommand(userID, fmt.Sprintf("/allow %d", strangerID))

		h.waitForMessageTo(userID, "лише для адміністраторів")
		if h.isAllowed(strangerID) {
			t.Errorf("a non-admin added someone to the allow list")
		}
	})

	t.Run("a malformed ID is explained, not stored", func(t *testing.T) {
		h.fake.SendCommand(adminID, "/allow abc")

		h.waitForMessageTo(adminID, "Вкажіть ID")
	})

	t.Run("the list shows admins and users", func(t *testing.T) {
		h.fake.SendCommand(adminID, "/users")

		list := h.waitForMessageTo(adminID, "Білий список")
		for _, id := range []int64{adminID, userID} {
			if !strings.Contains(list, fmt.Sprint(id)) {
				t.Errorf("the list lacks %d: %q", id, list)
			}
		}
	})

	t.Run("a removed user is locked out again", func(t *testing.T) {
		h.fake.SendCommand(adminID, fmt.Sprintf("/deny %d", userID))

		h.waitForMessageTo(adminID, "прибрано")
		if h.isAllowed(userID) {
			t.Fatalf("the removed user is still in the store")
		}

		h.fake.SendCommand(userID, "/start")
		h.waitForMessageTo(userID, supportContact)
	})
}

// TestAllowListByUsername covers the other way onto the allow list: by tag.
// Every user of the fake carries the username in fakeUsername.
func TestAllowListByUsername(t *testing.T) {
	h := startBot(t)

	h.fake.SendCommand(strangerID, "/start")
	h.waitForMessageTo(strangerID, supportContact)

	// Telegram usernames are case-insensitive, and admins type them with "@".
	h.fake.SendCommand(adminID, "/allow @"+strings.ToUpper(fakeUsername))

	h.waitForMessageTo(adminID, "додано")
	if !h.isUsernameAllowed(fakeUsername) {
		t.Fatalf("the allowed username is not in the store")
	}

	t.Run("the holder of the username gets in", func(t *testing.T) {
		h.fake.SendCommand(strangerID, "/start")

		h.waitForMessageTo(strangerID, "факультет")
	})

	t.Run("the list shows the username", func(t *testing.T) {
		h.fake.SendCommand(adminID, "/users")

		h.waitForMessageTo(adminID, "@"+fakeUsername)
	})

	t.Run("a malformed username is explained, not stored", func(t *testing.T) {
		h.fake.SendCommand(adminID, "/allow @no")

		h.waitForMessageTo(adminID, "Вкажіть ID або тег")
		if h.isUsernameAllowed("no") {
			t.Errorf("a malformed username reached the store")
		}
	})

	t.Run("a removed username is locked out again", func(t *testing.T) {
		h.fake.SendCommand(adminID, "/deny @"+fakeUsername)

		h.waitForMessageTo(adminID, "прибрано")
		if h.isUsernameAllowed(fakeUsername) {
			t.Fatalf("the removed username is still in the store")
		}

		h.fake.SendCommand(strangerID, "/start")
		h.waitForMessages(strangerID, supportContact, 2)
	})
}

// TestPickGroupAndReadSchedule walks the whole path a new user takes: pick a
// group through the inline keyboards, then read the schedule from the menu.
func TestPickGroupAndReadSchedule(t *testing.T) {
	h := startBot(t)

	h.fake.SendCommand(adminID, "/start")

	// The greeting is what brings the menu keyboard under the input field.
	welcome, err := h.fake.WaitForCall("sendMessage", "text", "Вітаю", replyWait)
	if err != nil {
		t.Fatalf("no greeting: %v", err)
	}
	if keyboard := welcome.Text("reply_markup"); !strings.Contains(keyboard, "Сьогодні") {
		t.Errorf("the greeting does not carry the menu keyboard: %q", keyboard)
	}

	faculties, err := h.fake.WaitForCall("sendMessage", "text", "факультет", replyWait)
	if err != nil {
		t.Fatalf("no faculty picker: %v", err)
	}
	h.click(adminID, faculties, siteFacultyName)

	courses, err := h.fake.WaitForCall("editMessageText", "text", "курс", replyWait)
	if err != nil {
		t.Fatalf("no course picker: %v", err)
	}
	// Skipping answerCallbackQuery leaves the user's client spinning until it
	// times out, which no unit test notices and every user does.
	_, err = h.fake.WaitForCall("answerCallbackQuery", "", "", replyWait)
	if err != nil {
		t.Fatalf("the button was never answered: %v", err)
	}
	h.click(adminID, courses, fmt.Sprint(siteCourse))

	groups, err := h.fake.WaitForCall("editMessageText", "text", "групу", replyWait)
	if err != nil {
		t.Fatalf("no group picker: %v", err)
	}
	h.click(adminID, groups, siteGroupName)

	// The picker itself turns into the confirmation. A new message here would
	// leave a dead keyboard above it.
	saved, err := h.fake.WaitForCall("editMessageText", "text", "збережено", replyWait)
	if err != nil {
		t.Fatalf("the picker was not turned into a confirmation: %v", err)
	}
	if !strings.Contains(saved.Text("text"), siteGroupName) {
		t.Errorf("the confirmation does not name the group: %q", saved.Text("text"))
	}
	if saved.Int("message_id") != pickerMessageID {
		t.Errorf("the confirmation edited message %d, want the picker %d", saved.Int("message_id"), pickerMessageID)
	}
	if h.fake.IndexOf("sendMessage", "text", "збережено") >= 0 {
		t.Errorf("the confirmation was also sent as a new message")
	}
	if group := h.storedGroup(adminID); group != siteGroupID {
		t.Fatalf("stored group %d, want %d", group, siteGroupID)
	}

	t.Run("today from the button under the confirmation", func(t *testing.T) {
		h.click(adminID, saved, "Сьогодні")

		today := h.waitForCalls("editMessageText", "text", siteSubject, 1)
		if !strings.Contains(today.Text("text"), "09:00-10:20") {
			t.Errorf("today lacks the class time: %q", today.Text("text"))
		}
	})

	t.Run("today from the menu button", func(t *testing.T) {
		h.fake.SendText(adminID, "📅 Сьогодні")

		today := h.waitForMessageTo(adminID, siteSubject)
		for _, want := range []string{siteGroupName, "09:00-10:20", "101", "Тестовий Викладач"} {
			if !strings.Contains(today, want) {
				t.Errorf("today lacks %q: %q", want, today)
			}
		}
	})

	t.Run("the second read comes from the cache", func(t *testing.T) {
		before := h.site.timetables.Load()

		h.fake.SendCommand(adminID, "/today")
		h.waitForMessages(adminID, siteSubject, 2)

		if after := h.site.timetables.Load(); after != before {
			t.Errorf("the site was asked %d more times for a cached week", after-before)
		}
	})

	t.Run("the week from the menu button", func(t *testing.T) {
		h.fake.SendText(adminID, "🗓 Тиждень")

		week := h.waitForMessageTo(adminID, "Тиждень")
		if !strings.Contains(week, siteSubject) {
			t.Errorf("the week lacks the class: %q", week)
		}
	})

	t.Run("day navigation edits the message in place", func(t *testing.T) {
		day, err := h.fake.WaitForCall("sendMessage", "text", "09:00-10:20", replyWait)
		if err != nil {
			t.Fatalf("no day message to navigate from: %v", err)
		}
		h.click(adminID, day, "▶")

		// The second edit showing a class: the first came from the button
		// under the confirmation.
		h.waitForCalls("editMessageText", "text", siteSubject, 2)
	})
}

// TestSiteOutageIsExplained checks the bot says so when the schedule site is
// down, and recovers once it is back. A failure must not be cached.
func TestSiteOutageIsExplained(t *testing.T) {
	h := startBot(t)

	h.site.down.Store(true)
	h.fake.SendCommand(adminID, "/start")
	h.waitForMessageTo(adminID, "не відповідає")

	h.site.down.Store(false)
	h.fake.SendCommand(adminID, "/start")
	h.waitForMessageTo(adminID, "факультет")
}

// TestSurvivesTelegramErrors checks the bot keeps serving after the API fails
// under it. Provoking this is the thing a fake can do that the real API cannot.
func TestSurvivesTelegramErrors(t *testing.T) {
	h := startBot(t)

	h.fake.FailNext("sendMessage", 403, "Forbidden: bot was blocked by the user")
	h.fake.SendCommand(adminID, "/help")

	// The failure is swallowed; the next command must still be answered.
	h.fake.SendCommand(adminID, "/help")
	h.waitForMessages(adminID, "Розклад КНУБА", 2)
}

// harness is one bot process with everything it talks to.
type harness struct {
	t      *testing.T
	fake   *tgfake.Server
	site   *fakeSite
	bot    *tgfake.Process
	dbPath string
}

// startBot runs the bot against fresh fakes and a fresh store, so tests stay
// independent of each other.
func startBot(t *testing.T) *harness {
	t.Helper()

	fake := tgfake.New()
	t.Cleanup(fake.Close)

	site := newFakeSite()
	t.Cleanup(site.server.Close)

	cache := miniredis.RunT(t)

	dbPath := filepath.Join(t.TempDir(), "bot.db")

	bot, err := tgfake.StartProcess(binaryPath, map[string]string{
		"BOT_TOKEN": "123456:FAKE",
		// The bot must reach the fakes instead of Telegram and the real site.
		"TELEGRAM_API_URL": fake.URL(),
		"MKR_BASE_URL":     site.server.URL,
		"MKR_TIMEOUT":      "5s",
		"REDIS_URL":        "redis://" + cache.Addr() + "/0",
		"CACHE_TTL":        "1h",
		"ADMIN_IDS":        fmt.Sprint(adminID),
		"DB_PATH":          dbPath,
		"HTTP_ADDR":        "127.0.0.1:0",
		"LOG_LEVEL":        "debug",
	})
	if err != nil {
		t.Fatalf("start bot: %v", err)
	}
	t.Cleanup(bot.Stop)

	// The bot's own log separates "tried and failed" from "never tried", which
	// the API side alone cannot.
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("--- bot log ---\n%s", bot.Logs())
		}
	})

	err = bot.WaitForLog("telegram bot listening", replyWait)
	if err != nil {
		t.Fatalf("startup: %v\n%s", err, bot.Logs())
	}

	return &harness{t: t, fake: fake, site: site, bot: bot, dbPath: dbPath}
}

// waitForMessageTo blocks until the bot has sent chatID a message containing
// want, and returns it. Scoping by chat matters: with several users in one
// test, an unscoped check would accept a reply meant for somebody else.
func (h *harness) waitForMessageTo(chatID int64, want string) string {
	h.t.Helper()

	return h.waitForMessages(chatID, want, 1)
}

// waitForMessages blocks until the bot has sent chatID at least count messages
// containing want, and returns the latest.
func (h *harness) waitForMessages(chatID int64, want string, count int) string {
	h.t.Helper()

	deadline := time.Now().Add(replyWait)
	for {
		var matching []string
		for _, text := range h.fake.MessagesTo(chatID) {
			if strings.Contains(text, want) {
				matching = append(matching, text)
			}
		}
		if len(matching) >= count {
			return matching[len(matching)-1]
		}

		if time.Now().After(deadline) {
			h.t.Fatalf("chat %d got %d messages containing %q, want %d; it got: %q",
				chatID, len(matching), want, count, h.fake.MessagesTo(chatID))
		}
		time.Sleep(pollTick)
	}
}

// waitForCalls blocks until the bot has made at least count calls to method
// whose param contains want, and returns the latest.
func (h *harness) waitForCalls(method, param, want string, count int) tgfake.Call {
	h.t.Helper()

	deadline := time.Now().Add(replyWait)
	for {
		var matching []tgfake.Call
		for _, call := range h.fake.Calls() {
			if call.Method == method && strings.Contains(call.Text(param), want) {
				matching = append(matching, call)
			}
		}
		if len(matching) >= count {
			return matching[len(matching)-1]
		}

		if time.Now().After(deadline) {
			h.t.Fatalf("got %d %s calls with %s containing %q, want %d", len(matching), method, param, want, count)
		}
		time.Sleep(pollTick)
	}
}

// click taps the inline button of a recorded message whose label contains
// label, the way a user would: with the payload the bot itself put there.
func (h *harness) click(chatID int64, message tgfake.Call, label string) {
	h.t.Helper()

	var markup struct {
		InlineKeyboard [][]struct {
			Text string `json:"text"`
			Data string `json:"callback_data"`
		} `json:"inline_keyboard"`
	}

	err := json.Unmarshal([]byte(message.Text("reply_markup")), &markup)
	if err != nil {
		h.t.Fatalf("message has no readable keyboard: %v: %q", err, message.Text("reply_markup"))
	}

	for _, row := range markup.InlineKeyboard {
		for _, button := range row {
			if strings.Contains(button.Text, label) {
				h.fake.ClickButton(chatID, pickerMessageID, button.Data)
				return
			}
		}
	}

	h.t.Fatalf("no button labelled %q in %q", label, message.Text("reply_markup"))
}

// isAllowed reads the allow list straight out of the bot's store. Reading
// through the bot's own commands would only tell what the bot believes.
func (h *harness) isAllowed(id int64) bool {
	h.t.Helper()

	var count int
	h.queryRow(&count, `SELECT COUNT(*) FROM allowed_users WHERE telegram_id = ?`, id)

	return count > 0
}

// isUsernameAllowed reads the username half of the allow list from the store.
func (h *harness) isUsernameAllowed(username string) bool {
	h.t.Helper()

	var count int
	h.queryRow(&count, `SELECT COUNT(*) FROM allowed_usernames WHERE username = ?`, username)

	return count > 0
}

// storedGroup returns the group ID saved for a user, or 0 when there is none.
func (h *harness) storedGroup(id int64) int {
	h.t.Helper()

	var group int
	h.queryRow(&group, `SELECT COALESCE(MAX(group_id), 0) FROM users WHERE telegram_id = ?`, id)

	return group
}

func (h *harness) queryRow(dest any, query string, args ...any) {
	h.t.Helper()

	db, err := sql.Open("sqlite", h.dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		h.t.Fatalf("open bot store: %v", err)
	}
	defer db.Close()

	err = db.QueryRow(query, args...).Scan(dest)
	if err != nil {
		h.t.Fatalf("read bot store: %v", err)
	}
}

// binaryPath is built once for the whole package.
//
// It deliberately avoids t.TempDir(): that directory is removed when the test
// that created it ends, which would delete the binary out from under every
// later test.
var binaryPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "knubaschedulebot-offline")
	if err != nil {
		fmt.Fprintf(os.Stderr, "temp dir: %v\n", err)
		os.Exit(1)
	}

	binaryPath = filepath.Join(dir, "knubaschedulebot")

	build := exec.Command("go", "build", "-o", binaryPath, "../../cmd/knubaschedulebot")
	out, err := build.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "build: %v\n%s", err, out)
		os.RemoveAll(dir)
		os.Exit(1)
	}

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
