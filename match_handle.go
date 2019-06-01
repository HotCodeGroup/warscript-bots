package main

import (
	"context"
	"math"
	"net/http"
	"strconv"

	"github.com/HotCodeGroup/warscript-utils/models"
	"github.com/HotCodeGroup/warscript-utils/utils"
	"github.com/google/uuid"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
)

// GetMatch gets match full info by ID
func GetMatch(w http.ResponseWriter, r *http.Request) {
	logger := utils.GetLogger(r, logger, "GetMatch")
	errWriter := utils.NewErrorResponseWriter(w, logger)
	vars := mux.Vars(r)

	matchID, err := strconv.ParseInt(vars["match_id"], 10, 64)
	if err != nil {
		errWriter.WriteError(http.StatusNotFound, errors.Wrap(err, "wrong format match_id"))
		return
	}

	matchInfo, err := Matches.GetMatchByID(matchID)
	if err != nil {
		if errors.Cause(err) == utils.ErrNotExists {
			errWriter.WriteWarn(http.StatusNotFound, errors.Wrap(err, "match not exists"))
		} else {
			errWriter.WriteError(http.StatusInternalServerError, errors.Wrap(err, "get match method error"))
		}
		return
	}

	ids := []*models.UserID{
		{ID: matchInfo.Author1},
	}

	// второй игрок может быть нашим ботом
	if matchInfo.Author2.Valid {
		ids = append(ids, &models.UserID{ID: matchInfo.Author2.Int64})
	}

	users, err := authGPRC.GetUsersByIDs(context.Background(), &models.UserIDs{
		IDs: ids,
	})
	if err != nil || users == nil {
		errWriter.WriteWarn(http.StatusNotFound, errors.Wrap(err, "can't get users by grpc"))
		return
	}

	var ai1 *AuthorInfo
	var ai2 *AuthorInfo

	for j := 0; j < 2; j++ {
		if len(users.Users) > j {
			if users.Users[j].ID == matchInfo.Author1 {
				ai1 = &AuthorInfo{
					ID:        users.Users[j].ID,
					Username:  users.Users[j].Username,
					PhotoUUID: users.Users[j].PhotoUUID,
					Active:    users.Users[j].Active,
				}
			} else if matchInfo.Author2.Valid && users.Users[j].ID == matchInfo.Author2.Int64 {
				ai2 = &AuthorInfo{
					ID:        users.Users[j].ID,
					Username:  users.Users[j].Username,
					PhotoUUID: users.Users[j].PhotoUUID,
					Active:    users.Users[j].Active,
				}
			}
		}
	}

	resp := MatchFullInfo{
		MatchInfo: MatchInfo{
			ID:       matchInfo.ID,
			Result:   matchInfo.Result,
			GameSlug: matchInfo.GameSlug,
			Bot1ID:   matchInfo.Bot1,
			Bot2ID:   matchInfo.GetBot2(),
			Diff1:    matchInfo.Diff1,
			Diff2:    matchInfo.GetDiff2(),
			Author1:  ai1,
			Author2:  ai2,
		},
		Error:     matchInfo.GetError(),
		Timestamp: matchInfo.Timestamp,
	}

	if matchInfo.Result != 3 {
		resp.Replay = &Replay{
			Info:   matchInfo.Info,
			States: matchInfo.States,
			Winner: matchInfo.Result,
		}
	}

	var session *models.SessionPayload
	cookie, err := r.Cookie("JSESSIONID")
	if err == nil || cookie != nil {
		session, err = authGPRC.GetSessionInfo(r.Context(), &models.SessionToken{Token: cookie.Value})
		if err != nil {
			logger.Warnf("can't get session by token: %v", err)
		}
	}

	if session != nil {
		if resp.Author1 != nil && session.ID == resp.Author1.ID {
			bot, err := Bots.GetBotByID(resp.Bot1ID)
			if err != nil {
				logger.Errorf("can't get bot by id: %v", err)
			} else {
				resp.Code = bot.Code
			}
			if matchInfo.Error1.Valid {
				resp.Error = matchInfo.Error1.String
			}
		} else if resp.Author2 != nil && session.ID == resp.Author2.ID {
			bot, err := Bots.GetBotByID(resp.Bot2ID)
			if err != nil {
				logger.Errorf("can't get bot by id: %v", err)
			} else {
				resp.Code = bot.Code
			}
			if matchInfo.Error2.Valid {
				resp.Error = matchInfo.Error2.String
			}
		}
	}

	utils.WriteApplicationJSON(w, http.StatusOK, resp)
}

// GetMatchList получает список последних матчей
func GetMatchList(w http.ResponseWriter, r *http.Request) {
	logger := utils.GetLogger(r, logger, "GetMatchList")
	errWriter := utils.NewErrorResponseWriter(w, logger)

	authorUsername := r.URL.Query().Get("author")
	var err error
	var authorID int64 = -1
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
		since = math.MaxInt64
	}

	gameSlug := r.URL.Query().Get("game_slug")
	matches, err := Matches.GetMatchesByGameSlugAndAuthorID(authorID, gameSlug, limit, since)
	if err != nil {
		errWriter.WriteError(http.StatusInternalServerError, errors.Wrap(err, "get bot method error"))
		return
	}

	if len(matches) == 0 {
		emptyResp := make([]*MatchInfo, 0)
		utils.WriteApplicationJSON(w, http.StatusOK, emptyResp)
		return
	}

	// если мы выбираем только для одного юзера, то нет смысла ходить по сети
	var authorsSet map[int64]*models.InfoUser
	// фомируем массив из всех айдишников авторов ботов
	userIDsSet := make(map[int64]struct{})
	for _, match := range matches {
		userIDsSet[match.Author1] = struct{}{}
		if match.Author2.Valid {
			userIDsSet[match.Author2.Int64] = struct{}{}
		}
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

	respMatches := make([]*MatchInfo, len(matches))
	for i, match := range matches {
		var ai1 *AuthorInfo
		var ai2 *AuthorInfo

		// если мы выбираем только для одного юзера
		if protUser, ok := authorsSet[match.Author1]; ok {
			ai1 = &AuthorInfo{
				ID:        protUser.ID,
				Username:  protUser.Username,
				PhotoUUID: protUser.PhotoUUID,
				Active:    protUser.Active,
			}
		}

		if protUser, ok := authorsSet[match.Author2.Int64]; match.Author2.Valid && ok {
			ai2 = &AuthorInfo{
				ID:        protUser.ID,
				Username:  protUser.Username,
				PhotoUUID: protUser.PhotoUUID,
				Active:    protUser.Active,
			}
		}

		respMatches[i] = &MatchInfo{
			ID:       match.ID,
			Result:   match.Result,
			GameSlug: match.GameSlug,
			Diff1:    match.Diff1,
			Diff2:    match.GetDiff2(),
			Author1:  ai1,
			Author2:  ai2,
		}
	}

	utils.WriteApplicationJSON(w, http.StatusOK, respMatches)
}

// OpenWS отдаёт лидерборд
func OpenWS(w http.ResponseWriter, r *http.Request) {
	logger := utils.GetLogger(r, logger, "GetBotsList")
	errWriter := utils.NewErrorResponseWriter(w, logger)
	// info := SessionInfo(r)
	// if info == nil {
	// 	errWriter.WriteWarn(http.StatusUnauthorized, errors.New("session info is not presented"))
	// 	return
	// }

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
	wsClient := &BotVerifyClient{
		SessionID: sessionID,
		//UserID:    1,
		GameSlug: gameSlug,

		h:    h,
		conn: c,
		send: make(chan *BotStatusMessage),
	}
	wsClient.h.register <- wsClient

	go wsClient.WriteStatusUpdates()
	go wsClient.WaitForClose()
}
