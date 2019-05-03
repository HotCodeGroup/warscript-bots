package main

import (
	"strconv"

	"github.com/HotCodeGroup/warscript-utils/utils"
	"github.com/jackc/pgx"
	"github.com/jackc/pgx/pgtype"
	"github.com/pkg/errors"
)

var pgxConn *pgx.ConnPool

// BotAccessObject DAO for Bot model
type BotAccessObject interface {
	Create(b *BotModel) error
	SetBotVerifiedByID(botID int64, isActive bool) error
	GetBotsByGameSlugAndAuthorUsername(author, game string) ([]*BotModel, error)
}

// AccessObject implementation of BotAccessObject
type AccessObject struct{}

var Bots BotAccessObject

func init() {
	Bots = &AccessObject{}
}

// Bot mode for bots table
type BotModel struct {
	ID             pgtype.Int8
	Code           pgtype.Text
	Language       pgtype.Varchar
	IsActive       pgtype.Bool
	IsVerified     pgtype.Bool
	AuthorUsername pgtype.Varchar
	GameSlug       pgtype.Varchar
}

func (bd *AccessObject) Create(b *BotModel) error {
	tx, err := pgxConn.Begin()
	if err != nil {
		return errors.Wrap(err, "can not open bot create transaction")
	}
	//nolint: errcheck
	defer tx.Rollback()

	row := tx.QueryRow(`INSERT INTO bots (code, language, author_username, game_slug)
	 	VALUES ($1, $2, $3, $4) RETURNING id`,
		&b.Code, &b.Language, &b.AuthorUsername, &b.GameSlug)
	if err = row.Scan(&b.ID); err != nil {
		pgErr, ok := err.(pgx.PgError)
		if !ok {
			return errors.Wrap(err, "can not insert bot row")
		}
		if pgErr.Code == "23505" {
			return errors.Wrap(utils.ErrTaken, errors.Wrap(err, "code duplication").Error())
		}
		return errors.Wrap(pgErr, "can not insert bot row")
	}

	err = tx.Commit()
	if err != nil {
		return errors.Wrap(err, "can not commit bot create transaction")
	}

	return nil
}

func (bd *AccessObject) SetBotVerifiedByID(botID int64, isVerified bool) error {
	row := pgxConn.QueryRow(`UPDATE bots SET is_verified = $1 
									WHERE bots.id = $2 RETURNING bots.id;`, isVerified, botID)

	var id int64
	if err := row.Scan(&id); err != nil {
		if err == pgx.ErrNoRows {
			return errors.Wrap(utils.ErrNotExists, errors.Wrap(err, "now row to update").Error())
		}

		return errors.Wrap(err, "can not update bot row")
	}

	return nil
}

func (bd *AccessObject) GetBotsByGameSlugAndAuthorUsername(author, game string) ([]*BotModel, error) {
	args := []interface{}{}
	query := `SELECT b.id, b.code, b.language,
	b.is_active, b.is_verified, b.author_username, b.game_slug 
	FROM bots b`
	if author != "" {
		query += ` WHERE b.author_username = $1`
		args = append(args, author)
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
	query += ";"

	rows, err := pgxConn.Query(query, args...)
	if err != nil {
		return nil, errors.Wrap(err, "get bots by game slug and author id error")
	}
	defer rows.Close()

	bots := make([]*BotModel, 0)
	for rows.Next() {
		bot := &BotModel{}
		err = rows.Scan(&bot.ID, &bot.Code,
			&bot.Language, &bot.IsActive, &bot.IsVerified,
			&bot.AuthorUsername, &bot.GameSlug)
		if err != nil {
			return nil, errors.Wrap(err, "get bots by game slug and author id scan bot error")
		}
		bots = append(bots, bot)
	}

	return bots, nil
}
