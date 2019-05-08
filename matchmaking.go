package main

import (
	"encoding/json"
	"math"
	"sync"
	"time"

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
			for i := 0; i < len(bots); i += 1 {
				nextI := i + 1
				if nextI == len(bots) {
					nextI = 0
				}

				if bots[i].Language == bots[nextI].Language && bots[i].AuthorID != bots[nextI].AuthorID {
					// делаем RPC запрос
					events, err := sendForVerifyRPC(&TestTask{
						Code1:    bots[i].Code.String,
						Code2:    bots[nextI].Code.String,
						GameSlug: gameSlug, // так как citext, то ориджинал слаг в gameInfo
						Language: Lang(bots[i].Language.String),
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
	gameSlug := bot1.GameSlug.String

	logger := logger.WithFields(logrus.Fields{
		"bot_id1": bot1.ID.Int,
		"bot_id2": bot2.ID.Int,
		"method":  "processTestingStatus",
	})

	status := ""
	for event := range events {
		logger.Infof("Processing [%s]", event.Type)
		switch event.Type {
		case "status":
			upd := &TesterStatusUpdate{}
			err := json.Unmarshal(event.Body, upd)
			if err != nil {
				logger.Error(errors.Wrap(err, "can not unmarshal update status body"))
				continue
			}

			body, _ := json.Marshal(&MatchStatus{
				Bot1ID:    bot1.ID.Int,
				Bot2ID:    bot2.ID.Int,
				Author1ID: bot1.AuthorID.Int,
				Author2ID: bot2.AuthorID.Int,
				NewStatus: upd.NewStatus,
			})

			broadcast <- &BotStatusMessage{
				AuthorID: bot1.AuthorID.Int,
				GameSlug: gameSlug,
				Body:     body,
				Type:     "match_status",
			}

			broadcast <- &BotStatusMessage{
				AuthorID: bot2.AuthorID.Int,
				GameSlug: gameSlug,
				Body:     body,
				Type:     "match_status",
			}

			status = upd.NewStatus
		case "result":
			res := &TesterStatusResult{}
			err := json.Unmarshal(event.Body, res)
			if err != nil {
				logger.Error(errors.Wrap(err, "can not unmarshal result status body"))
				continue
			}

			newScore1, newScore2 := newRatings(bot1.Score.Int, bot2.Score.Int, res.Winner)

			body, _ := json.Marshal(&MatchResult{
				Bot1ID:    bot1.ID.Int,
				Bot2ID:    bot2.ID.Int,
				Author1ID: bot1.AuthorID.Int,
				Author2ID: bot2.AuthorID.Int,
				NewScore1: newScore1,
				NewScore2: newScore2,
				Winner:    res.Winner,
			})

			broadcast <- &BotStatusMessage{
				AuthorID: bot1.AuthorID.Int,
				GameSlug: gameSlug,
				Body:     body,
				Type:     "match",
			}

			broadcast <- &BotStatusMessage{
				AuthorID: bot2.AuthorID.Int,
				GameSlug: gameSlug,
				Body:     body,
				Type:     "match",
			}

			err = Bots.SetBotScoreByID(bot1.ID.Int, newScore1)
			if err != nil {
				logger.Error(errors.Wrap(err, "can't update bot1 score"))
				continue
			}
			err = Bots.SetBotScoreByID(bot2.ID.Int, newScore2)
			if err != nil {
				logger.Error(errors.Wrap(err, "can't update bot2 score"))
				continue
			}
		case "error":
			res := &TesterStatusError{}
			err := json.Unmarshal(event.Body, res)
			if err != nil {
				logger.Error(errors.Wrap(err, "can not unmarshal result status body"))
				continue
			}

			logger.Info(res.Error)

			body, _ := json.Marshal(&MatchResult{
				Bot1ID:    bot1.ID.Int,
				Bot2ID:    bot2.ID.Int,
				Author1ID: bot1.AuthorID.Int,
				Author2ID: bot2.AuthorID.Int,
				NewScore1: bot1.Score.Int,
				NewScore2: bot2.Score.Int,
				Winner:    -1,
			})

			broadcast <- &BotStatusMessage{
				AuthorID: bot1.AuthorID.Int,
				GameSlug: gameSlug,
				Body:     body,
				Type:     "match_error",
			}

			broadcast <- &BotStatusMessage{
				AuthorID: bot2.AuthorID.Int,
				GameSlug: gameSlug,
				Body:     body,
				Type:     "match_error",
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
