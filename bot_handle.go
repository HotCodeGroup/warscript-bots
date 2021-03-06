package main

import (
	"context"
	"net/http"
	"strconv"

	"github.com/HotCodeGroup/warscript-utils/middlewares"
	"github.com/HotCodeGroup/warscript-utils/models"
	"github.com/HotCodeGroup/warscript-utils/utils"
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

// CreateBot создание бота в базе данных + отправка его на проверку
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

	// проверяем, что такой юзер есть, и достаём username
	userInfo, err := authGPRC.GetUserByID(context.Background(), &models.UserID{ID: info.ID})
	if err != nil {
		errWriter.WriteError(http.StatusInternalServerError, errors.Wrap(err, "can not find user by session token"))
		return
	}

	bot := &BotModel{
		Code:     form.Code,
		Language: string(form.Language),
		GameSlug: gameInfo.Slug,
		AuthorID: userInfo.ID,
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
			Author: &AuthorInfo{
				ID:        userInfo.ID,
				Username:  userInfo.Username,
				PhotoUUID: userInfo.PhotoUUID,
				Active:    userInfo.Active,
			},
			ID:         bot.ID,
			IsActive:   bot.IsActive,
			IsVerified: bot.IsVerified,
			GameSlug:   bot.GameSlug,
			Score:      bot.Score,
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
	go processVerifyingStatus(bot.ID, info.ID, bot.GameSlug, h.broadcast, events)
	utils.WriteApplicationJSON(w, http.StatusOK, botFull)
}

// GetBotsList получение списка ботов
func GetBotsList(w http.ResponseWriter, r *http.Request) {
	logger := utils.GetLogger(r, logger, "GetBotsList")
	errWriter := utils.NewErrorResponseWriter(w, logger)

	authorUsername := r.URL.Query().Get("author")

	var authorID int64 = -1

	var err error
	var userInfo *models.InfoUser
	if authorUsername != "" {
		userInfo, err = authGPRC.GetUserByUsername(context.Background(), &models.Username{Username: authorUsername})
		if err != nil {
			if errors.Cause(err) == utils.ErrNotExists {
				utils.WriteApplicationJSON(w, http.StatusOK, []*Bot{})
			} else {
				errWriter.WriteError(http.StatusInternalServerError, errors.Wrap(err, "can not find user by username"))
			}

			return
		}

		authorID = userInfo.ID
	}

	limitS := r.URL.Query().Get("limit")
	sinceS := r.URL.Query().Get("since")

	limit, err := strconv.ParseInt(limitS, 10, 64)
	if err != nil {
		limit = 10
	}
	since, err := strconv.ParseInt(sinceS, 10, 64)
	if err != nil {
		since = 0
	}

	gameSlug := r.URL.Query().Get("game_slug")
	bots, err := Bots.GetBotsByGameSlugAndAuthorID(authorID, gameSlug, limit, since)
	if err != nil {
		errWriter.WriteError(http.StatusInternalServerError, errors.Wrap(err, "get bot method error"))
		return
	}

	if len(bots) == 0 {
		emptyResp := make([]*Bot, 0)
		utils.WriteApplicationJSON(w, http.StatusOK, emptyResp)
		return
	}

	// если мы выбираем только для одного юзера, то нет смысла ходить по сети
	var authorsSet map[int64]*models.InfoUser
	if authorID == -1 && userInfo == nil {
		// фомируем массив из всех айдишников авторов ботов
		userIDsSet := make(map[int64]struct{})
		for _, bot := range bots {
			userIDsSet[bot.AuthorID] = struct{}{}
		}
		userIDsM := &models.UserIDs{
			IDs: make([]*models.UserID, 0, len(userIDsSet)),
		}
		for id := range userIDsSet {
			userIDsM.IDs = append(userIDsM.IDs, &models.UserID{ID: id})
		}

		// делаем запрос
		authorsInfo, err := authGPRC.GetUsersByIDs(context.Background(), userIDsM)
		if err != nil {
			errWriter.WriteError(http.StatusInternalServerError, errors.Wrap(err, "can not find user by ids"))
			return
		}

		// формиурем хеш ответов
		authorsSet = make(map[int64]*models.InfoUser, len(authorsInfo.Users))
		for _, authorInfo := range authorsInfo.Users {
			authorsSet[authorInfo.ID] = authorInfo
		}
	}

	respBots := make([]*Bot, len(bots))
	for i, bot := range bots {
		var ai *AuthorInfo

		// если мы выбираем только для одного юзера
		if authorID != -1 && userInfo != nil {
			ai = &AuthorInfo{
				ID:        userInfo.ID,
				Username:  userInfo.Username,
				PhotoUUID: userInfo.PhotoUUID,
				Active:    userInfo.Active,
			}
		} else if protUser, ok := authorsSet[bot.AuthorID]; ok {
			ai = &AuthorInfo{
				ID:        protUser.ID,
				Username:  protUser.Username,
				PhotoUUID: protUser.PhotoUUID,
				Active:    protUser.Active,
			}
		}

		respBots[i] = &Bot{
			Author:     ai,
			ID:         bot.ID,
			GameSlug:   bot.GameSlug,
			IsActive:   bot.IsActive,
			IsVerified: bot.IsVerified,
			Score:      bot.Score,
		}
	}

	utils.WriteApplicationJSON(w, http.StatusOK, respBots)
}
