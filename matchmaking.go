package main

import (
	"encoding/json"
	"math"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var (
	botsLimit = int64(100)
	gameSlugs = []string{"pong"}
)

func startMatchmaking() {
	for {
		for _, gameSlug := range gameSlugs {
			bots, err := Bots.GetBotsForTesting(botsLimit, gameSlug)
			if err != nil {
				logger.Error(errors.Wrap(err, "can't get bots for testing "+gameSlug))
				continue
			}
			if len(bots) < 1 {
				continue
			}

			for i := 0; i < len(bots); i += 2 {
				if bots[i].Language == bots[i+1].Language {
					// делаем RPC запрос
					events, err := sendForVerifyRPC(&TestTask{
						Code1:    bots[i].Code.String,
						Code2:    bots[i+1].Code.String,
						GameSlug: gameSlug, // так как citext, то ориджинал слаг в gameInfo
						Language: Lang(bots[i].Language.String),
					})
					if err != nil {
						logger.Error(errors.Wrap(err, "failed to call testing rpc"))
						continue
					}
					// запускаем обработчик ответа RPC
					go processTestingStatus(bots[i], bots[i+1], h.broadcast, events)
				}
			}
		}
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
			}

			broadcast <- &BotStatusMessage{
				AuthorID: bot2.AuthorID.Int,
				GameSlug: gameSlug,
				Body:     body,
			}

			status = upd.NewStatus
		case "result":
			res := &TesterStatusResult{}
			err := json.Unmarshal(event.Body, res)
			if err != nil {
				logger.Error(errors.Wrap(err, "can not unmarshal result status body"))
				continue
			}

			newStatus := "Not Verifyed\n"
			if res.Winner == 1 {
				newStatus = "Verifyed\n"
			}

			broadcast <- &BotStatusMessage{
				BotID:     botID,
				AuthorID:  authorID,
				GameSlug:  gameSlug,
				NewStatus: newStatus,
			}

			err = Bots.SetBotVerifiedByID(botID, res.Winner == 1)
			if err != nil {
				logger.Error(errors.Wrap(err, "can update bot active status"))
				continue
			}

			status = newStatus
		case "error":
			res := &TesterStatusError{}
			err := json.Unmarshal(event.Body, res)
			if err != nil {
				logger.Error(errors.Wrap(err, "can not unmarshal result status body"))
				continue
			}

			logger.Info(res.Error)
			newStatus := "Not Verifyed. Error!\n"
			broadcast <- &BotStatusMessage{
				BotID:     botID,
				AuthorID:  authorID,
				GameSlug:  gameSlug,
				NewStatus: newStatus,
			}

			err = Bots.SetBotVerifiedByID(botID, false)
			if err != nil {
				logger.Error(errors.Wrap(err, "can update bot active status"))
				continue
			}

			status = newStatus
		default:
			logger.Error(errors.New("can not process unknown status type"))
		}

		logger.Infof("Processing [%s]: new status: %s", event.Type, status)
	}
}

func newRatings(sc1, sc2, winner int) (int, int) {
	if winner == 0 {
		return sc1 + int(40*(0.5-expVal(sc2, sc1))), sc2 + int(40*(0.5-expVal(sc1, sc2)))
	} else if winner == 1 {
		return sc1 + int(40*(1-expVal(sc2, sc1))), sc2 - int(40*(expVal(sc1, sc2)))
	} else if winner == 2 {
		return sc1 - int(40*(expVal(sc2, sc1))), sc2 + int(40*(1-expVal(sc1, sc2)))
	}

	return sc1, sc2
}

func expVal(sc1, sc2 int) float64 {
	return 1.0 / (1 + math.Pow(10, float64(sc1-sc2)/400))
}
