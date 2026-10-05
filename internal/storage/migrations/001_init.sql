-- users holds the group each Telegram user picked. The group's name, faculty
-- and course are stored with its ID because the schedule site needs all of
-- them to return a timetable.
CREATE TABLE users (
    telegram_id INTEGER PRIMARY KEY,
    group_id    INTEGER NOT NULL,
    group_name  TEXT    NOT NULL,
    faculty_id  INTEGER NOT NULL,
    course      INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);

-- allowed_users holds the Telegram users admins let into the bot. Admins
-- themselves come from config and are not stored here.
CREATE TABLE allowed_users (
    telegram_id INTEGER PRIMARY KEY,
    added_by    INTEGER NOT NULL,
    added_at    INTEGER NOT NULL
);
