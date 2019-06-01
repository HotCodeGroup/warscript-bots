package main

import (
	"database/sql"
	"log"
	"strconv"

	"github.com/HotCodeGroup/warscript-utils/utils"
	"github.com/pkg/errors"

	"github.com/lib/pq"
)

// BotAccessObject DAO for Bot model
type BotAccessObject interface {
	Create(b *BotModel) error
	SetBotVerifiedByID(botID int64, isActive bool) error
	SetBotScoreByID(botID int64, newScore int64) error
	GetBotByID(botID int64) (*BotModel, error)
	GetBotsByGameSlugAndAuthorID(authorID int64, game string, limit, since int64) ([]*BotModel, error)
	GetBotsForTesting(N int64, game string) ([]*BotModel, error)
}

// AccessObject implementation of BotAccessObject
type AccessObject struct{}

// Bots объект для обращения с моделью bot
var Bots BotAccessObject

func init() {
	Bots = &AccessObject{}
}

// BotModel model for bots table
type BotModel struct {
	ID          int64
	Code        string
	Language    string
	IsActive    bool
	IsVerified  bool
	AuthorID    int64
	GameSlug    string
	Score       int64
	GamesPlayed int64
}

// Create создание записи о боте в базе данных
func (bd *AccessObject) Create(b *BotModel) error {
	tx, err := pqConn.Begin()
	if err != nil {
		return errors.Wrapf(utils.ErrInternal, "can not open bot create transaction: %s", err.Error())
	}
	//nolint: errcheck
	defer tx.Rollback()

	row := tx.QueryRow(`INSERT INTO bots (code, language, author_id, game_slug)
	 	VALUES ($1, $2, $3, $4) RETURNING id`,
		&b.Code, &b.Language, &b.AuthorID, &b.GameSlug)
	if err = row.Scan(&b.ID); err != nil {
		pgErr, ok := err.(pq.Error)
		log.Printf("%v %v", pgErr, ok)
		if !ok {
			return errors.Wrapf(utils.ErrInternal, "create bot row error: %v", err)
		}

		if pgErr.Code == "23505" {
			return errors.Wrapf(utils.ErrTaken, "code duplication: %v", err)
		}

		return errors.Wrapf(utils.ErrInternal, "create bot row error: %v", pgErr)
	}

	err = tx.Commit()
	if err != nil {
		return errors.Wrapf(utils.ErrInternal, "can not commit bot create transaction: %v", err)
	}

	return nil
}

// SetBotVerifiedByID установка флага проверки по ID
func (bd *AccessObject) SetBotVerifiedByID(botID int64, isVerified bool) error {
	row := pqConn.QueryRow(`UPDATE bots SET is_verified = $1 
									WHERE bots.id = $2 RETURNING bots.id;`, isVerified, botID)

	var id int64
	if err := row.Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return errors.Wrapf(utils.ErrNotExists, "now row to update: %v", err)
		}

		return errors.Wrapf(utils.ErrInternal, "can not update bot row: %v", err)
	}

	return nil
}

// SetBotScoreByID установка очков для бота по ID
func (bd *AccessObject) SetBotScoreByID(botID, newScore int64) error {
	_, err := pqConn.Exec(`UPDATE bots SET score = $1 
									WHERE bots.id = $2;`, newScore, botID)
	if err != nil {
		return errors.Wrapf(utils.ErrInternal, "can not update bot row: %v", err)
	}

	return nil
}

// GetBotByID получение бота по его идентификатору
func (bd *AccessObject) GetBotByID(botID int64) (*BotModel, error) {
	row := pqConn.QueryRow(`SELECT b.id, b.code, b.language,
	b.is_active, b.is_verified, b.author_id, b.game_slug, b.score, b.games_played 
	FROM bots b WHERE b.id=$1`, botID)

	bot := &BotModel{}
	err := row.Scan(&bot.ID, &bot.Code,
		&bot.Language, &bot.IsActive, &bot.IsVerified,
		&bot.AuthorID, &bot.GameSlug, &bot.Score, &bot.GamesPlayed)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.Wrapf(utils.ErrNotExists, "bot with this id does not exist: %v", err)
		}

		return nil, errors.Wrapf(utils.ErrInternal, "can not get bot by id: %v", err)
	}

	return bot, nil
}

// GetBotsByGameSlugAndAuthorID получение спика ботов для какой-либо игры и/или пользователя
func (bd *AccessObject) GetBotsByGameSlugAndAuthorID(authorID int64, game string,
	limit, since int64) ([]*BotModel, error) {
	args := []interface{}{}
	query := `SELECT b.id, b.code, b.language,
	b.is_active, b.is_verified, b.author_id, b.game_slug, b.score, b.games_played 
	FROM bots b`
	if authorID > 0 {
		query += ` WHERE b.author_id = $1`
		args = append(args, authorID)
	}

	if game != "" {
		if len(args) == 0 {
			query += ` WHERE`
		} else {
			query += ` AND`
		}
		query += ` b.game_slug = $`
		query += strconv.Itoa(len(args) + 1)
		args = append(args, game)
	}
	query += " ORDER BY b.score DESC LIMIT $"
	query += strconv.Itoa(len(args) + 1)
	args = append(args, limit)
	query += " OFFSET $"
	query += strconv.Itoa(len(args) + 1)
	args = append(args, since)
	query += ";"

	rows, err := pqConn.Query(query, args...)
	if err != nil {
		return nil, errors.Wrapf(utils.ErrInternal, "get bots by game slug and author id error: %v", err)
	}
	defer rows.Close()

	bots := make([]*BotModel, 0)
	for rows.Next() {
		bot := &BotModel{}
		err = rows.Scan(&bot.ID, &bot.Code,
			&bot.Language, &bot.IsActive, &bot.IsVerified,
			&bot.AuthorID, &bot.GameSlug, &bot.Score, &bot.GamesPlayed)
		if err != nil {
			return nil, errors.Wrapf(utils.ErrInternal, "get bots by game slug and author id scan bot error: %v", err)
		}
		bots = append(bots, bot)
	}

	return bots, nil
}

// GetBotsForTesting выборка ботов для новой серии матчев
func (bd *AccessObject) GetBotsForTesting(n int64, game string) ([]*BotModel, error) {
	query := `(SELECT distinct * FROM (SELECT b.id, b.code, b.language,
	b.is_active, b.is_verified, b.author_id, b.game_slug, b.score, b.games_played
	FROM bots b WHERE b.is_verified = true AND b.game_slug = $1 AND b.games_played > 0 ORDER BY random() LIMIT $2) l) 
	UNION
	(SELECT b.id, b.code, b.language,
	b.is_active, b.is_verified, b.author_id, b.game_slug, b.score, b.games_played
	FROM bots b WHERE b.is_verified = true AND b.game_slug = $1 AND b.games_played = 0)`

	rows, err := pqConn.Query(query, game, n)
	if err != nil {
		return nil, errors.Wrapf(utils.ErrInternal, "get bots for testing error: %v", err)
	}
	defer rows.Close()

	bots := make([]*BotModel, 0)
	for rows.Next() {
		bot := &BotModel{}
		err = rows.Scan(&bot.ID, &bot.Code,
			&bot.Language, &bot.IsActive, &bot.IsVerified,
			&bot.AuthorID, &bot.GameSlug, &bot.Score, &bot.GamesPlayed)
		if err != nil {
			return nil, errors.Wrapf(utils.ErrInternal, "get bots for testing scan bot error: %v", err)
		}
		bots = append(bots, bot)
	}

	return bots, nil
}
