package main

import (
	"database/sql"
	"reflect"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/HotCodeGroup/warscript-utils/utils"
	"github.com/lib/pq"
	"github.com/pkg/errors"
)

func TestCreateOK(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO bots").
		WithArgs("111", "JS", 123, "pong").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectCommit()

	pqConn = db
	Bots = &AccessObject{}

	b := &BotModel{
		Code:     "111",
		Language: "JS",
		AuthorID: 123,
		GameSlug: "pong",
	}

	if err = Bots.Create(b); err != nil {
		t.Errorf("TestCreateOK got unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("TestCreateOK there were unfulfilled expectations: %s", err)
	}
}

func botCreateError(t *testing.T, db *sql.DB,
	mock sqlmock.Sqlmock, expectedError error) {
	pqConn = db
	Bots = &AccessObject{}

	b := &BotModel{
		Code:     "111",
		Language: "JS",
		AuthorID: 123,
		GameSlug: "pong",
	}

	err := Bots.Create(b)
	if errors.Cause(err) != expectedError {
		t.Errorf("botCreateError got unexpected error: %v, expected: %v", err, expectedError)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("botCreateError there were unfulfilled expectations: %s", err)
	}
}

func TestBotCreateBeginError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	mock.ExpectBegin().WillReturnError(sql.ErrConnDone)
	botCreateError(t, db, mock, utils.ErrInternal)
}

func TestBotCreateInternalError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO bots").
		WithArgs("111", "JS", 123, "pong").
		WillReturnError(sql.ErrConnDone)
	mock.ExpectRollback()

	botCreateError(t, db, mock, utils.ErrInternal)
}

func TestBotCreateTakenError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO bots").
		WithArgs("111", "JS", 123, "pong").
		WillReturnError(pq.Error{Code: "23505"})
	mock.ExpectRollback()

	botCreateError(t, db, mock, utils.ErrTaken)
}

func TestBotCreateInternalPQError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO bots").
		WithArgs("111", "JS", 123, "pong").
		WillReturnError(pq.Error{Code: "1337"})
	mock.ExpectRollback()

	botCreateError(t, db, mock, utils.ErrInternal)
}

func TestBotCreateCommitError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO bots").
		WithArgs("111", "JS", 123, "pong").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectCommit().WillReturnError(sql.ErrConnDone)

	botCreateError(t, db, mock, utils.ErrInternal)
}

func TestSetBotVerifiedByIDok(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	mock.ExpectQuery("UPDATE bots").
		WithArgs(true, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	pqConn = db
	Bots = &AccessObject{}

	if err = Bots.SetBotVerifiedByID(1, true); err != nil {
		t.Errorf("TestSetBotVerifiedByIDok got unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("TestSetBotVerifiedByIDok there were unfulfilled expectations: %s", err)
	}
}

func setBotVerifiedByIDError(t *testing.T, db *sql.DB,
	mock sqlmock.Sqlmock, expectedError error) {
	pqConn = db
	Bots = &AccessObject{}

	err := Bots.SetBotVerifiedByID(1, true)
	if errors.Cause(err) != expectedError {
		t.Errorf("setBotVerifiedByIDError got unexpected error: %v, expected: %v", err, expectedError)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("setBotVerifiedByIDError there were unfulfilled expectations: %s", err)
	}
}

func TestSetBotVerifiedByIDNotExists(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	mock.ExpectQuery("UPDATE bots").
		WithArgs(true, 1).WillReturnError(sql.ErrNoRows)

	setBotVerifiedByIDError(t, db, mock, utils.ErrNotExists)
}

func TestSetBotVerifiedByIDInternal(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	mock.ExpectQuery("UPDATE bots").
		WithArgs(true, 1).WillReturnError(sql.ErrConnDone)

	setBotVerifiedByIDError(t, db, mock, utils.ErrInternal)
}

func TestSetBotScoreByIDok(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	mock.ExpectExec("UPDATE bots").
		WithArgs(15, 1).
		WillReturnResult(sqlmock.NewResult(1, 1))

	pqConn = db
	Bots = &AccessObject{}

	if err = Bots.SetBotScoreByID(1, 15); err != nil {
		t.Errorf("TestSetBotScoreByIDok got unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("TestSetBotScoreByIDok there were unfulfilled expectations: %s", err)
	}
}

func TestSetBotScoreByIDInternal(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	mock.ExpectExec("UPDATE bots").
		WithArgs(15, 1).
		WillReturnError(sql.ErrConnDone)

	pqConn = db
	Bots = &AccessObject{}

	if err = Bots.SetBotScoreByID(1, 15); errors.Cause(err) != utils.ErrInternal {
		t.Errorf("TestSetBotScoreByIDInternal got unexpected error: %v; expected %v", err, utils.ErrInternal)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("TestSetBotScoreByIDInternal there were unfulfilled expectations: %s", err)
	}
}

//nolint: dupl
func TestGetBotsByGameSlugAndAuthorIDok(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT").
		WithArgs(1, "pong").
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "language",
			"is_active", "is_verified", "author_id", "game_slug", "score", "games_played"}).
			AddRow(1, "a=5;", "JS", true, true, 1, "pong", 500, 1))

	pqConn = db
	Bots = &AccessObject{}

	botModel, err := Bots.GetBotsByGameSlugAndAuthorID(1, "pong")
	if err != nil {
		t.Errorf("GetBotsByGameSlugAndAuthorID got unexpected error: %v", err)
	}

	expected := []*BotModel{
		{
			ID:          1,
			Code:        "a=5;",
			Language:    "JS",
			IsActive:    true,
			IsVerified:  true,
			AuthorID:    1,
			GameSlug:    "pong",
			Score:       500,
			GamesPlayed: 1,
		},
	}

	if !reflect.DeepEqual(botModel, expected) {
		t.Errorf("GetBotsByGameSlugAndAuthorID got unexpected result: %v; expected: %v", botModel, expected)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("GetBotsByGameSlugAndAuthorID there were unfulfilled expectations: %s", err)
	}
}

func TestGetBotsByGameSlugAndAuthorIDInternal(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT").
		WithArgs(1, "pong").
		WillReturnError(sql.ErrConnDone)

	pqConn = db
	Bots = &AccessObject{}

	_, err = Bots.GetBotsByGameSlugAndAuthorID(1, "pong")
	if errors.Cause(err) != utils.ErrInternal {
		t.Errorf("TestGetBotsByGameSlugAndAuthorIDInternal got unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("TestGetBotsByGameSlugAndAuthorIDInternal there were unfulfilled expectations: %s", err)
	}
}

func TestGetBotsByGameSlugAndAuthorIDScanInternal(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT").
		WithArgs("pong").
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "language",
			"is_active", "is_verified", "author_id", "game_slug", "score", "games_played"}).
			AddRow("kek", "a=5;", "JS", true, true, 1, "pong", 500, 1))

	pqConn = db
	Bots = &AccessObject{}

	_, err = Bots.GetBotsByGameSlugAndAuthorID(0, "pong")
	if errors.Cause(err) != utils.ErrInternal {
		t.Errorf("TestGetBotsByGameSlugAndAuthorIDInternal got unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("GetBotsByGameSlugAndAuthorID there were unfulfilled expectations: %s", err)
	}
}

//nolint: dupl
func TestGetBotsForTestingOK(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT").
		WithArgs("pong", 2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "language",
			"is_active", "is_verified", "author_id", "game_slug", "score", "games_played"}).
			AddRow(1, "a=5;", "JS", true, true, 1, "pong", 500, 1))

	pqConn = db
	Bots = &AccessObject{}

	botModel, err := Bots.GetBotsForTesting(2, "pong")
	if err != nil {
		t.Errorf("GetBotsByGameSlugAndAuthorID got unexpected error: %v", err)
	}

	expected := []*BotModel{
		{
			ID:          1,
			Code:        "a=5;",
			Language:    "JS",
			IsActive:    true,
			IsVerified:  true,
			AuthorID:    1,
			GameSlug:    "pong",
			Score:       500,
			GamesPlayed: 1,
		},
	}

	if !reflect.DeepEqual(botModel, expected) {
		t.Errorf("GetBotsByGameSlugAndAuthorID got unexpected result: %v; expected: %v", botModel, expected)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("GetBotsByGameSlugAndAuthorID there were unfulfilled expectations: %s", err)
	}
}

func TestGetBotsForTestingInternal(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT").
		WithArgs("pong", 2).
		WillReturnError(sql.ErrConnDone)

	pqConn = db
	Bots = &AccessObject{}

	_, err = Bots.GetBotsForTesting(2, "pong")
	if errors.Cause(err) != utils.ErrInternal {
		t.Errorf("TestGetBotsByGameSlugAndAuthorIDInternal got unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("TestGetBotsByGameSlugAndAuthorIDInternal there were unfulfilled expectations: %s", err)
	}
}

func TestGetBotsForTestingScanInternal(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT").
		WithArgs("pong", 2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "language",
			"is_active", "is_verified", "author_id", "game_slug", "score", "games_played"}).
			AddRow("kek", "a=5;", "JS", true, true, 1, "pong", 500, 1))

	pqConn = db
	Bots = &AccessObject{}

	_, err = Bots.GetBotsForTesting(2, "pong")
	if errors.Cause(err) != utils.ErrInternal {
		t.Errorf("TestGetBotsByGameSlugAndAuthorIDInternal got unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("TestGetBotsByGameSlugAndAuthorIDInternal there were unfulfilled expectations: %s", err)
	}
}
