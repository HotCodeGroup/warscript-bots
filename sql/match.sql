CREATE EXTENSION IF NOT EXISTS citext;

DROP TABLE IF EXISTS "matches";
CREATE TABLE "matches"
(
	id BIGSERIAL NOT NULL
		CONSTRAINT match_pk
			PRIMARY KEY,

	game_slug citext CONSTRAINT game_slug_empty NOT NULL CHECK ( game_slug <> '' ),
	info BYTEA,
	states BYTEA,
	error TEXT,
	result INTEGER NOT NULL,
	time TIMESTAMP WITHOUT TIME ZONE DEFAULT now(),


	bot_1 BIGINT NOT NULL REFERENCES bots (id) ON DELETE NO ACTION,
	error_1 TEXT,
	author_1 BIGINT NOT NULL,
	log_1 BYTEA,
	diff_1 BIGINT NOT NULL,

	bot_2 BIGINT REFERENCES bots (id) ON DELETE NO ACTION,
	error_2 TEXT,
	author_2 BIGINT,
	log_2 BYTEA,
	diff_2 BIGINT
);

ALTER TABLE matches OWNER TO warscript_bots_user;