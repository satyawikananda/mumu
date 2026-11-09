CREATE TABLE sources (
	id              TEXT PRIMARY KEY,
	kind            TEXT NOT NULL CHECK (kind IN
		('builtin', 'remote_playlist', 'local_playlist', 'custom_stream')),
	identity        TEXT NOT NULL,
	location        TEXT NOT NULL,
	display_name    TEXT NOT NULL DEFAULT '',
	enabled         INTEGER NOT NULL DEFAULT 1,
	created_at      INTEGER NOT NULL,
	updated_at      INTEGER NOT NULL,
	last_refresh_at INTEGER,
	refresh_etag    TEXT,
	refresh_error   TEXT,
	UNIQUE (kind, identity)
);

CREATE TABLE channels (
	id          INTEGER PRIMARY KEY,
	source_id   TEXT NOT NULL REFERENCES sources (id) ON DELETE CASCADE,
	provider_id TEXT NOT NULL,
	name        TEXT NOT NULL,
	stream_url  TEXT NOT NULL,
	logo_url    TEXT,
	country     TEXT,
	category    TEXT,
	language    TEXT,
	is_live     INTEGER NOT NULL DEFAULT 1,
	updated_at  INTEGER NOT NULL,
	UNIQUE (source_id, provider_id)
);

CREATE TABLE favorites (
	channel_id INTEGER PRIMARY KEY REFERENCES channels (id) ON DELETE CASCADE,
	created_at INTEGER NOT NULL
);

CREATE TABLE recent_channels (
	channel_id INTEGER PRIMARY KEY REFERENCES channels (id) ON DELETE CASCADE,
	played_at  INTEGER NOT NULL,
	play_count INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE settings (
	key        TEXT PRIMARY KEY,
	value      TEXT NOT NULL,
	updated_at INTEGER NOT NULL
);

CREATE INDEX channels_source_name ON channels (source_id, name);
CREATE INDEX channels_name ON channels (name);
CREATE INDEX channels_country ON channels (country);
CREATE INDEX channels_category ON channels (category);
CREATE INDEX recent_played_at ON recent_channels (played_at DESC);
