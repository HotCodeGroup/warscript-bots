package main

import (
	"context"
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

	users, err := authGPRC.GetUsersByIDs(context.Background(), &models.UserIDs{
		IDs: []*models.UserID{
			&models.UserID{ID: matchInfo.Author1},
			&models.UserID{ID: matchInfo.Author2},
		},
	})
	if err != nil {
		errWriter.WriteWarn(http.StatusNotFound, errors.Wrap(err, "can't get users by grpc"))
	}

	resp := MatchFullInfo{
		ID:        matchInfo.ID,
		States:    matchInfo.States,
		Error:     matchInfo.GetError(),
		Result:    matchInfo.Result,
		Diff1:     matchInfo.Diff1,
		Diff2:     matchInfo.Diff2,
		Timestamp: matchInfo.Timestamp,
		GameSlug:  matchInfo.GameSlug,
		Bot1ID:    matchInfo.Bot1,
		Bot2ID:    matchInfo.Bot2,
		User1: AuthorInfo{
			ID:        users.Users[0].ID,
			Username:  users.Users[0].Username,
			PhotoUUID: users.Users[0].PhotoUUID,
			Active:    users.Users[0].Active,
		},
		User2: AuthorInfo{
			ID:        users.Users[1].ID,
			Username:  users.Users[1].Username,
			PhotoUUID: users.Users[1].PhotoUUID,
			Active:    users.Users[1].Active,
		},
	}

	authenticated := true
	cookie, err := r.Cookie("JSESSIONID")
	if err != nil || cookie == nil {
		authenticated = false
	}

	session, err := authGPRC.GetSessionInfo(r.Context(), &models.SessionToken{Token: cookie.Value})
	if err != nil {
		authenticated = false
	}

	if authenticated {
		if session.ID == resp.User1.ID {
			bot, err := Bots.GetBotByID(resp.User1.ID)
			if err != nil {
				logger.Errorf("can't get bot by id")
			} else {
				resp.Code = bot.Code
			}
		} else if session.ID == resp.User2.ID {
			bot, err := Bots.GetBotByID(resp.User2.ID)
			if err != nil {
				logger.Errorf("can't get bot by id")
			} else {
				resp.Code = bot.Code
			}
			resp.Code = bot.Code
		}
	}

	utils.WriteApplicationJSON(w, http.StatusOK, resp)
}
