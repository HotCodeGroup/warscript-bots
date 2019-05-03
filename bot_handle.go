package main

import (
	"context"
	"net/http"

	"github.com/HotCodeGroup/warscript-utils/middlewares"
	"github.com/HotCodeGroup/warscript-utils/models"
	"github.com/HotCodeGroup/warscript-utils/utils"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/pgtype"
	"github.com/pkg/errors"
)

// SessionInfo достаёт инфу о юзере из контекстаs
func SessionInfo(r *http.Request) *models.SessionPayload {
	if rv := r.Context().Value(middlewares.SessionInfoKey); rv != nil {
		if rInfo, ok := rv.(*models.SessionPayload); ok {
			return rInfo
		}
	}

	return nil
}

func CreateBot(w http.ResponseWriter, r *http.Request) {
	logger := utils.GetLogger(r, logger, "CreateBot")
	errWriter := utils.NewErrorResponseWriter(w, logger)
	info := SessionInfo(r)
	if info == nil {
		errWriter.WriteWarn(http.StatusUnauthorized, errors.New("session info is not presented"))
		return
	}

	form := &BotUpload{}
	err := utils.DecodeBodyJSON(r.Body, form)
	if err != nil {
		errWriter.WriteWarn(http.StatusBadRequest, errors.Wrap(err, "decode body error"))
		return
	}

	if err = form.Validate(); err != nil {
		// уверены в преобразовании
		errWriter.WriteValidationError(err.(*utils.ValidationError))
		return
	}

	// проверяем, что такая игра есть, и достаём оригинальный slug
	gameInfo, err := gamesGPRC.GetGameBySlug(context.Background(), &models.GameSlug{Slug: form.GameSlug})
	if err != nil {
		if errors.Cause(err) == utils.ErrNotExists {
			errWriter.WriteValidationError(&utils.ValidationError{
				"game_slug": utils.ErrNotExists.Error(),
			})
			return
		}

		errWriter.WriteError(http.StatusInternalServerError, errors.Wrap(err, "bot create error"))
		return
	}

	// проверяем, что такая юзер есть, и достаём username
	userInfo, err := authGPRC.GetUserByID(context.Background(), &models.UserID{ID: info.ID})
	if err != nil {
		errWriter.WriteError(http.StatusInternalServerError, errors.Wrap(err, "can not find user by session token"))
		return
	}

	bot := &BotModel{
		Code:           pgtype.Text{String: form.Code, Status: pgtype.Present},
		Language:       pgtype.Varchar{String: string(form.Language), Status: pgtype.Present},
		GameSlug:       pgtype.Varchar{String: gameInfo.Slug, Status: pgtype.Present},
		AuthorUsername: pgtype.Varchar{String: userInfo.Username, Status: pgtype.Present},
	}

	if err = Bots.Create(bot); err != nil {
		if errors.Cause(err) == utils.ErrTaken {
			errWriter.WriteValidationError(&utils.ValidationError{
				"code": utils.ErrTaken.Error(),
			})
			return
		}

		errWriter.WriteError(http.StatusInternalServerError, errors.Wrap(err, "bot create error"))
		return
	}

	botFull := BotFull{
		Bot: Bot{
			ID:             bot.ID.Int,
			AuthorUsername: bot.AuthorUsername.String,
			IsActive:       bot.IsActive.Bool,
			IsVerified:     bot.IsVerified.Bool,
			GameSlug:       bot.GameSlug.String,
		},
		Code:     form.Code,
		Language: form.Language,
	}

	// делаем RPC запрос
	events, err := sendForVerifyRPC(&TestTask{
		Code1:    form.Code,
		Code2:    gameInfo.BotCode,
		GameSlug: gameInfo.Slug, // так как citext, то ориджинал слаг в gameInfo
		Language: form.Language,
	})
	if err != nil {
		errWriter.WriteError(http.StatusInternalServerError, errors.Wrap(err, "can not call verify rpc"))
		return
	}

	// запускаем обработчик ответа RPC
	go processTestingStatus(bot.ID.Int, info.ID, bot.GameSlug.String, h.broadcast, events)
	utils.WriteApplicationJSON(w, http.StatusOK, botFull)
}

// GetBotsList TODO: author_id parameter
func GetBotsList(w http.ResponseWriter, r *http.Request) {
	logger := utils.GetLogger(r, logger, "GetBotsList")
	errWriter := utils.NewErrorResponseWriter(w, logger)

	authorUsername := r.URL.Query().Get("author")
	gameSlug := r.URL.Query().Get("game_slug")
	var err error
	bots, err := Bots.GetBotsByGameSlugAndAuthorUsername(authorUsername, gameSlug)
	if err != nil {
		errWriter.WriteError(http.StatusInternalServerError, errors.Wrap(err, "get bot method error"))
		return
	}

	respBots := make([]*Bot, len(bots))
	for i, bot := range bots {
		respBots[i] = &Bot{
			ID:             bot.ID.Int,
			GameSlug:       bot.GameSlug.String,
			AuthorUsername: bot.AuthorUsername.String,
			IsActive:       bot.IsActive.Bool,
			IsVerified:     bot.IsVerified.Bool,
		}
	}

	utils.WriteApplicationJSON(w, http.StatusOK, respBots)
}

func OpenVerifyWS(w http.ResponseWriter, r *http.Request) {
	logger := utils.GetLogger(r, logger, "GetBotsList")
	errWriter := utils.NewErrorResponseWriter(w, logger)
	info := SessionInfo(r)
	if info == nil {
		errWriter.WriteWarn(http.StatusUnauthorized, errors.New("session info is not presented"))
		return
	}

	gameSlug := r.URL.Query().Get("game_slug")
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // мы уже прошли слой CORS
		},
	}
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		errWriter.WriteError(http.StatusInternalServerError, errors.Wrap(err, "upgrade to websocket error"))
		return
	}

	sessionID := uuid.New().String()
	verifyClient := &BotVerifyClient{
		SessionID: sessionID,
		UserID:    info.ID,
		//UserID:    1,
		GameSlug: gameSlug,

		h:    h,
		conn: c,
		send: make(chan *BotVerifyStatusMessage),
	}
	verifyClient.h.register <- verifyClient

	go verifyClient.WriteStatusUpdates()
	go verifyClient.WaitForClose()
}
