package main

import (
	"context"
	"net/http"

	"github.com/HotCodeGroup/warscript-utils/models"
	"github.com/HotCodeGroup/warscript-utils/utils"
	"github.com/pkg/errors"
)

func GetMatchList(w http.ResponseWriter, r *http.Request) {
	logger := utils.GetLogger(r, logger, "GetMatchList")
	errWriter := utils.NewErrorResponseWriter(w, logger)

	gameSlug := r.URL.Query().Get("game_slug")
	matches, err := Matches.GetMatchesByGameSlugAndAuthorID(-1, gameSlug)
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
		userIDsSet[match.Author2] = struct{}{}
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

		if protUser, ok := authorsSet[match.Author2]; ok {
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
