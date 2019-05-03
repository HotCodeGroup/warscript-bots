DROP TYPE IF EXISTS LANG CASCADE;
CREATE TYPE LANG AS ENUM ('JS');

DROP TABLE IF EXISTS "bots";
CREATE TABLE "bots"
(
	id BIGSERIAL NOT NULL
		CONSTRAINT bot_pk
			PRIMARY KEY,
	code TEXT CONSTRAINT code_empty NOT NULL CHECK ( code <> '' ),
	language LANG NOT NULL,
	is_active BOOLEAN NOT NULL DEFAULT FALSE,
    is_verified BOOLEAN NOT NULL DEFAULT FALSE,
	author_username citext NOT NULL REFERENCES users (username) ON DELETE CASCADE,
	game_slug citext NOT NULL REFERENCES games (slug) ON DELETE CASCADE,

	CONSTRAINT unique_code UNIQUE (code, language, author_username, game_slug)
);