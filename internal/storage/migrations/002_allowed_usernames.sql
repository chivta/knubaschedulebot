-- allowed_usernames holds the Telegram usernames admins let into the bot,
-- stored lowercase and without the leading "@". An entry follows the
-- username, not the account: whoever holds the username gets access.
CREATE TABLE allowed_usernames (
    username TEXT    PRIMARY KEY,
    added_by INTEGER NOT NULL,
    added_at INTEGER NOT NULL
);
