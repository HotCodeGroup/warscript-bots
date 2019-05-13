package main

import (
	"database/sql"
	"strconv"
	"time"

	"github.com/HotCodeGroup/warscript-utils/utils"
	"github.com/pkg/errors"
)

// MatchAccessObject DAO for Match model
type MatchAccessObject interface {
	Create(b *MatchModel) error
	GetMatchByID(matchID int64) (*MatchModel, error)
	GetMatchesByGameSlugAndAuthorID(authorID int64, gameSlug string, limit int64, since int64) ([]*MatchModel, error)
}

// AccessObject implementation of BotAccessObject
type MatchObject struct{}

// Bots объект для обращения с моделью bot
var Matches MatchAccessObject

func init() {
	Matches = &MatchObject{}
}

// Bot mode for bots table
type MatchModel struct {
	ID        int64
	States    []byte
	Error     sql.NullString
	Result    int
	Timestamp time.Time
	GameSlug  string
	Bot1      int64
	Author1   int64
	Log1      []byte
	Diff1     int64
	Bot2      sql.NullInt64
	Author2   sql.NullInt64
	Log2      []byte
	Diff2     sql.NullInt64
}

func (m *MatchModel) GetError() string {
	if m.Error.Valid {
		return m.Error.String
	}

	return ""
}

func (m *MatchModel) GetBot2() int64 {
	if m.Bot2.Valid {
		return m.Bot2.Int64
	}

	return 0
}

func (m *MatchModel) GetAuthor2() int64 {
	if m.Author2.Valid {
		return m.Author2.Int64
	}

	return 0
}

func (m *MatchModel) GetDiff2() int64 {
	if m.Diff2.Valid {
		return m.Diff2.Int64
	}

	return 0
}

func (o *MatchObject) Create(m *MatchModel) error {
	tx, err := pqConn.Begin()
	if err != nil {
		return errors.Wrapf(utils.ErrInternal, "can not open match create transaction: %s", err.Error())
	}
	//nolint: errcheck
	defer tx.Rollback()

	m.Timestamp = time.Now()
	row := tx.QueryRow(`INSERT INTO matches (game_slug, states, error, result, time, bot_1, author_1, log_1, diff_1, bot_2, author_2, log_2, diff_2)
	 	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13) RETURNING id, time`,
		&m.GameSlug, &m.States, &m.Error, &m.Result, &m.Timestamp, &m.Bot1,
		&m.Author1, &m.Log1, &m.Diff1, &m.Bot2, &m.Author2, &m.Log2, &m.Diff2)
	if err = row.Scan(&m.ID, &m.Timestamp); err != nil {
		return errors.Wrapf(utils.ErrInternal, "create match row error: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		return errors.Wrapf(utils.ErrInternal, "can not commit match create transaction: %v", err)
	}

	return nil
}

func (o *MatchObject) GetMatchByID(matchID int64) (*MatchModel, error) {
	row := pqConn.QueryRow(`SELECT m.id, m.game_slug, m.states, m.error, m.result, m.time, m.bot_1, m.author_1,
       m.log_1, m.diff_1, m.bot_2, m.author_2, m.log_2, m.diff_2 FROM matches m WHERE m.id=$1`, matchID)

	m := &MatchModel{}
	err := row.Scan(&m.ID, &m.GameSlug, &m.States, &m.Error, &m.Result, &m.Timestamp,
		&m.Bot1, &m.Author1, &m.Log1, &m.Diff1, &m.Bot2, &m.Author2, &m.Log2, &m.Diff2)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.Wrapf(utils.ErrNotExists, "match with this id does not exist: %v", err)
		}

		return nil, errors.Wrapf(utils.ErrInternal, "can not get match by id: %v", err)
	}

	return m, nil
}

func (o *MatchObject) GetMatchesByGameSlugAndAuthorID(authorID int64, gameSlug string, limit int64, since int64) ([]*MatchModel, error) {
	args := []interface{}{since}

	query := `SELECT m.id, m.game_slug, m.states, m.error, m.result, m.time, m.bot_1, m.author_1,
       m.log_1, m.diff_1, m.bot_2, m.author_2, m.log_2, m.diff_2 FROM matches m WHERE m.id < $1`
	if authorID > 0 {
		query += ` AND (m.author_1 = $2 OR m.author_2 = $2)`
		args = append(args, authorID)
	}

	if gameSlug != "" {
		query += ` AND m.game_slug = $`
		query += strconv.Itoa(len(args) + 1)
		args = append(args, gameSlug)
	}
	query += " ORDER BY m.id DESC LIMIT $"
	query += strconv.Itoa(len(args) + 1)
	args = append(args, limit)

	query += ";"

	rows, err := pqConn.Query(query, args...)
	if err != nil {
		return nil, errors.Wrapf(utils.ErrInternal, "get matches by game slug and author id error: %v", err)
	}
	defer rows.Close()

	matches := make([]*MatchModel, 0)
	for rows.Next() {
		m := &MatchModel{}
		err := rows.Scan(&m.ID, &m.GameSlug, &m.States, &m.Error, &m.Result, &m.Timestamp,
			&m.Bot1, &m.Author1, &m.Log1, &m.Diff1, &m.Bot2, &m.Author2, &m.Log2, &m.Diff2)
		if err != nil {
			return nil, errors.Wrapf(utils.ErrInternal, "get bots by game slug and author id scan bot error: %v", err)
		}
		matches = append(matches, m)
	}

	return matches, nil
}
