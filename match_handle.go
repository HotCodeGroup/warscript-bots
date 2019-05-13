package main

import (
	"context"
	"math"
	"net/http"
	"strconv"

	"github.com/HotCodeGroup/warscript-utils/models"
	"github.com/HotCodeGroup/warscript-utils/utils"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
)

//GetMatch gets match full info by ID
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
		&models.UserID{ID: matchInfo.Author1},
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
	if len(users.Users) > 0 {
		if users.Users[0].ID == matchInfo.Author1 {
			ai1 = &AuthorInfo{
				ID:        users.Users[0].ID,
				Username:  users.Users[0].Username,
				PhotoUUID: users.Users[0].PhotoUUID,
				Active:    users.Users[0].Active,
			}
		} else if matchInfo.Author2.Valid && users.Users[0].ID == matchInfo.Author2.Int64 {
			ai2 = &AuthorInfo{
				ID:        users.Users[0].ID,
				Username:  users.Users[0].Username,
				PhotoUUID: users.Users[0].PhotoUUID,
				Active:    users.Users[0].Active,
			}
		}
	}

	if len(users.Users) > 1 {
		if users.Users[1].ID == matchInfo.Author1 {
			ai1 = &AuthorInfo{
				ID:        users.Users[1].ID,
				Username:  users.Users[1].Username,
				PhotoUUID: users.Users[1].PhotoUUID,
				Active:    users.Users[1].Active,
			}
		} else if matchInfo.Author2.Valid && users.Users[1].ID == matchInfo.Author2.Int64 {
			ai2 = &AuthorInfo{
				ID:        users.Users[1].ID,
				Username:  users.Users[1].Username,
				PhotoUUID: users.Users[1].PhotoUUID,
				Active:    users.Users[1].Active,
			}
		}
	}

	resp := MatchFullInfo{
		ID:        matchInfo.ID,
		States:    matchInfo.States,
		Error:     matchInfo.GetError(),
		Result:    matchInfo.Result,
		Diff1:     matchInfo.Diff1,
		Diff2:     matchInfo.GetDiff2(),
		Timestamp: matchInfo.Timestamp,
		GameSlug:  matchInfo.GameSlug,
		Bot1ID:    matchInfo.Bot1,
		Bot2ID:    matchInfo.GetBot2(),
		Author1:   ai1,
		Author2:   ai2,
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
		} else if resp.Author2 != nil && session.ID == resp.Author2.ID {
			bot, err := Bots.GetBotByID(resp.Bot2ID)
			if err != nil {
				logger.Errorf("can't get bot by id: %v", err)
			} else {
				resp.Code = bot.Code
			}
			resp.Code = bot.Code
		}
	}

	utils.WriteApplicationJSON(w, http.StatusOK, resp)
}

func GetMatchList(w http.ResponseWriter, r *http.Request) {
	logger := utils.GetLogger(r, logger, "GetMatchList")
	errWriter := utils.NewErrorResponseWriter(w, logger)

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
	matches, err := Matches.GetMatchesByGameSlugAndAuthorID(-1, gameSlug, limit, since)
	if err != nil {
		errWriter.WriteError(http.StatusInternalServerError, errors.Wrap(err, "get bot method error"))
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
			Author1:  ai1,
			Author2:  ai2,
		}
	}

	utils.WriteApplicationJSON(w, http.StatusOK, respMatches)
}
