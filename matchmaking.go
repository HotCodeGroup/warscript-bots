package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"sync"
	"time"

	"github.com/HotCodeGroup/warscript-utils/models"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var (
	botsLimit = int64(100)
	gameSlugs = []string{"pong"}
)

func startMatchmaking() {
	for {
		timer := time.NewTimer(10 * time.Second)
		for _, gameSlug := range gameSlugs {
			bots, err := Bots.GetBotsForTesting(botsLimit, gameSlug)
			if err != nil {
				logger.Error(errors.Wrap(err, "can't get bots for testing "+gameSlug))
				continue
			}
			if len(bots) < 1 {
				continue
			}

			wg := sync.WaitGroup{}
			for i := 0; i < len(bots); i++ {
				nextI := i + 1
				if nextI == len(bots) {
					nextI = 0
				}

				if bots[i].Language == bots[nextI].Language && bots[i].AuthorID != bots[nextI].AuthorID {
					// делаем RPC запрос
					events, err := sendForVerifyRPC(&TestTask{
						Code1:    bots[i].Code,
						Code2:    bots[nextI].Code,
						GameSlug: gameSlug, // так как citext, то ориджинал слаг в gameInfo
						Language: Lang(bots[i].Language),
					})
					if err != nil {
						logger.Error(errors.Wrap(err, "failed to call testing rpc"))
						continue
					}
					// запускаем обработчик ответа RPC

					go func(b1 *BotModel, b2 *BotModel, ev <-chan *TesterStatusQueue) {
						defer wg.Done()

						wg.Add(1)
						processTestingStatus(b1, b2, h.broadcast, ev)
					}(bots[i], bots[nextI], events)
				}
			}

			wg.Wait()
		}
		<-timer.C
	}
}

func processTestingStatus(bot1, bot2 *BotModel,
	broadcast chan<- *BotStatusMessage, events <-chan *TesterStatusQueue) {
	gameSlug := bot1.GameSlug

	logger := logger.WithFields(logrus.Fields{
		"bot_id1": bot1.ID,
		"bot_id2": bot2.ID,
		"method":  "processTestingStatus",
	})

	status := ""
	for event := range events {
		logger.Infof("Processing [%s]", event.Type)
		switch event.Type {
		case "result":
			res := &TesterStatusResult{}
			err := json.Unmarshal(event.Body, res)
			if err != nil {
				logger.Error(errors.Wrap(err, "can not unmarshal result status body"))
				continue
			}

			// Обновили ботов
			newScore1, newScore2 := newRatings(bot1.Score, bot2.Score, res.Winner)
			err = Bots.SetBotScoreByID(bot1.ID, newScore1)
			if err != nil {
				logger.Error(errors.Wrap(err, "can't update bot1 score"))
				continue
			}
			err = Bots.SetBotScoreByID(bot2.ID, newScore2)
			if err != nil {
				logger.Error(errors.Wrap(err, "can't update bot2 score"))
				continue
			}

			// сохранили матч
			m := &MatchModel{
				Info:     res.Info,
				States:   res.States,
				Result:   res.Winner,
				GameSlug: gameSlug,

				Bot1:    bot1.ID,
				Author1: bot1.AuthorID,
				Diff1:   newScore1 - bot1.Score,

				Bot2:    sql.NullInt64{Int64: bot2.ID, Valid: true},
				Author2: sql.NullInt64{Int64: bot2.AuthorID, Valid: true},
				Diff2:   sql.NullInt64{Int64: newScore2 - bot2.Score, Valid: true},
			}
			err = Matches.Create(m)
			if err != nil {
				logger.Error(errors.Wrap(err, "can not save match"))
				continue
			}

			// делаем запрос
			userIDsM := &models.UserIDs{
				IDs: []*models.UserID{
					{ID: bot1.AuthorID},
					{ID: bot2.AuthorID},
				},
			}
			authorsInfo, err := authGPRC.GetUsersByIDs(context.Background(), userIDsM)
			if err != nil {
				logger.Error(errors.Wrap(err, "can not get author info"))
				continue
			}

			// формиурем хеш ответов
			authorsSet := make(map[int64]*models.InfoUser, len(authorsInfo.Users))
			for _, authorInfo := range authorsInfo.Users {
				authorsSet[authorInfo.ID] = authorInfo
			}

			var ai1 *AuthorInfo
			var ai2 *AuthorInfo

			// вдруг какая-то инфа не пришла
			if protUser, ok := authorsSet[bot1.AuthorID]; ok {
				ai1 = &AuthorInfo{
					ID:        protUser.ID,
					Username:  protUser.Username,
					PhotoUUID: protUser.PhotoUUID,
					Active:    protUser.Active,
				}
			}

			if protUser, ok := authorsSet[bot2.AuthorID]; ok {
				ai2 = &AuthorInfo{
					ID:        protUser.ID,
					Username:  protUser.Username,
					PhotoUUID: protUser.PhotoUUID,
					Active:    protUser.Active,
				}
			}

			body, err := json.Marshal(&MatchInfo{
				ID:        m.ID,
				Result:    m.Result,
				GameSlug:  m.GameSlug,
				Author1:   ai1,
				Author2:   ai2,
				Bot1ID:    bot1.ID,
				Bot2ID:    bot2.ID,
				NewScore1: newScore1,
				NewScore2: newScore2,
				Diff1:     newScore1 - bot1.Score,
				Diff2:     newScore2 - bot2.Score,
			})
			if err != nil {
				logger.Error(errors.Wrap(err, "can marshal match info"))
				continue
			}

			// т.к. инфу ещё нужно отправить двум юзерам
			broadcast <- &BotStatusMessage{
				AuthorID: bot1.AuthorID,
				GameSlug: gameSlug,
				Body:     body,
				Type:     "match",
			}

			broadcast <- &BotStatusMessage{
				Private:  true,
				AuthorID: bot2.AuthorID,
				GameSlug: gameSlug,
				Body:     body,
				Type:     "match",
			}

			bodyVK1, err := json.Marshal(&NotifyMatchMessage{
				BotID:    bot1.ID,
				GameSlug: gameSlug,
				MatchID:  m.ID,
				Diff:     newScore1 - bot1.Score,
			})
			if err != nil {
				logger.Error(errors.Wrap(err, "can not marshal body user1"))
				continue
			}

			_, err = notifyGRPC.SendNotify(context.Background(), &models.Message{
				Type: "match",
				User: bot1.AuthorID,
				Game: gameSlug,
				Body: bodyVK1,
			})
			if err != nil {
				logger.Error(errors.Wrap(err, "can send notify to user1"))
				continue
			}

			bodyVK2, err := json.Marshal(&NotifyMatchMessage{
				BotID:    bot2.ID,
				GameSlug: gameSlug,
				MatchID:  m.ID,
				Diff:     newScore2 - bot2.Score,
			})
			if err != nil {
				logger.Error(errors.Wrap(err, "can not marshal body user2"))
				continue
			}
			_, err = notifyGRPC.SendNotify(context.Background(), &models.Message{
				Type: "match",
				User: bot2.AuthorID,
				Game: gameSlug,
				Body: bodyVK2,
			})
			if err != nil {
				logger.Error(errors.Wrap(err, "can send notify to user1"))
				continue
			}

		case "error":
			res := &TesterStatusError{}
			err := json.Unmarshal(event.Body, res)
			if err != nil {
				logger.Error(errors.Wrap(err, "can not unmarshal error status body"))
				continue
			}

			logger.Infof("Match error: %s", res.Error)
			err = Matches.Create(&MatchModel{
				Result:   3,
				Error:    sql.NullString{String: res.Error, Valid: true},
				GameSlug: gameSlug,

				Bot1:    bot1.ID,
				Author1: bot1.AuthorID,
				Diff1:   0,

				Bot2:    sql.NullInt64{Int64: bot2.ID, Valid: true},
				Author2: sql.NullInt64{Int64: bot2.AuthorID, Valid: true},
				Diff2:   sql.NullInt64{Int64: 0, Valid: true},
			})
			if err != nil {
				logger.Error(errors.Wrap(err, "can not save match"))
				continue
			}

		default:
			logger.Error(errors.New("can not process unknown status type"))
		}

		logger.Infof("Processing [%s]: new status: %s", event.Type, status)
	}
}

func newRatings(sc1, sc2 int64, winner int) (int64, int64) {
	if winner == 0 {
		return sc1 + int64(40*(0.5-expVal(sc2, sc1))), sc2 + int64(40*(0.5-expVal(sc1, sc2)))
	} else if winner == 1 {
		return sc1 + int64(40*(1-expVal(sc2, sc1))), sc2 - int64(40*(expVal(sc1, sc2)))
	} else if winner == 2 {
		return sc1 - int64(40*(expVal(sc2, sc1))), sc2 + int64(40*(1-expVal(sc1, sc2)))
	}

	return sc1, sc2
}

func expVal(sc1, sc2 int64) float64 {
	return 1.0 / (1 + math.Pow(10, float64(sc1-sc2)/400))
}
