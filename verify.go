package main

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/HotCodeGroup/warscript-utils/models"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/streadway/amqp"

	"github.com/sirupsen/logrus"
)

const (
	testerQueueName = "tester_rpc_queue"
)

// TesterStatusQueue сообщение полученное из очереди задач
type TesterStatusQueue struct {
	Type string          `json:"type"`
	Body json.RawMessage `json:"body"`
}

// TesterStatusUpdate обновление статуса полученное из очереди задач
type TesterStatusUpdate struct {
	NewStatus string `json:"new_status"`
}

// TesterStatusError ошибка полученная из очереди задач
type TesterStatusError struct {
	Error string `json:"error"`
}

// TesterStatusResult результат матча полученный из очереди задач
type TesterStatusResult struct {
	Info   json.RawMessage `json:"info"`
	States json.RawMessage `json:"states"`
	Winner int             `json:"result"`
	Error1 string          `json:"error_1"`
	Error2 string          `json:"error_2"`
	Logs1  json.RawMessage `json:"logs_1"`
	Logs2  json.RawMessage `json:"logs_2"`
}

// TestTask представление задачи на проверку, которое кладётся в очередь задач
type TestTask struct {
	Code1    string `json:"code1"`
	Code2    string `json:"code2"`
	GameSlug string `json:"game_slug"`
	Language Lang   `json:"lang"`
}

func sendForVerifyRPC(task *TestTask) (<-chan *TesterStatusQueue, error) {
	respQ, err := rabbitChannel.QueueDeclare(
		"", // пакет amqp сам сгенерит
		false,
		true,
		false, // удаляем после того, как процедура отработала
		false,
		nil,
	)
	if err != nil {
		return nil, errors.Wrap(err, "can not create queue for responses")
	}

	requestUUID := uuid.New().String()
	resps, err := rabbitChannel.Consume(
		respQ.Name,
		requestUUID,
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return nil, errors.Wrap(err, "can not register a consumer")
	}

	body, err := json.Marshal(task)
	if err != nil {
		return nil, errors.Wrap(err, "can not marshal bot info")
	}

	err = rabbitChannel.Publish(
		"",
		testerQueueName,
		false,
		false,
		amqp.Publishing{
			ContentType:   "application/json",
			CorrelationId: requestUUID,
			ReplyTo:       respQ.Name,
			Body:          body,
		},
	)
	if err != nil {
		return nil, errors.Wrap(err, "can not publish a message")
	}

	events := make(chan *TesterStatusQueue)
	go func(in <-chan amqp.Delivery, out chan<- *TesterStatusQueue, corrID string) {
		for resp := range in {
			if corrID != resp.CorrelationId {
				continue
			}

			testerResp := &TesterStatusQueue{}
			err := json.Unmarshal(resp.Body, testerResp)
			if err != nil {
				logger.WithField("method", "sendForVerifyRPC goroutine").Error(errors.Wrap(err, "unmarshal tester response error"))
				continue
			}
			out <- testerResp

			if testerResp.Type == "result" || testerResp.Type == "error" {
				// отцепились от очереди -- она удалилась
				err = rabbitChannel.Cancel(
					corrID,
					false,
				)
				if err != nil {
					logger.WithField("method", "sendForVerifyRPC goroutine").Error(errors.Wrap(err, "queue cancel error"))
				}
			}
		}

		close(out)

	}(resps, events, requestUUID)

	return events, nil
}

func processVerifyingStatus(botID, authorID int64, gameSlug string,
	broadcast chan<- *BotStatusMessage, events <-chan *TesterStatusQueue) {

	logger := logger.WithFields(logrus.Fields{
		"bot_id": botID,
		"method": "processVerifyingStatus",
	})

	status := ""
	for event := range events {
		logger.Infof("Processing [%s]", event.Type)
		switch event.Type {
		case "status":
			continue
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

			body, _ := json.Marshal(&BotStatus{
				BotID:     botID,
				NewStatus: newStatus,
			})

			broadcast <- &BotStatusMessage{
				AuthorID: authorID,
				GameSlug: gameSlug,
				Body:     body,
				Type:     "verify",
			}

			var diff int64
			if res.Winner == 1 || res.Winner == 0 {
				err = Bots.SetBotVerifiedByID(botID, true)
				if err != nil {
					logger.Error(errors.Wrap(err, "can update bot verified status"))
					continue
				}

				err = Bots.SetBotScoreByID(botID, 400)
				if err != nil {
					logger.Error(errors.Wrap(err, "can update bot verified status"))
					continue
				}

				diff = 400
			}

			m := &MatchModel{
				Info:     res.Info,
				States:   res.States,
				Result:   res.Winner,
				GameSlug: gameSlug,
				Bot1:     botID,
				Error1:   sql.NullString{String: res.Error1, Valid: res.Error1 != ""},
				Author1:  authorID,
				Diff1:    diff,
			}
			err = Matches.Create(m)
			if err != nil {
				logger.Error(errors.Wrap(err, "can not save match"))
				continue
			}

			authorInfo, err := authGPRC.GetUserByID(context.Background(), &models.UserID{ID: authorID})
			if err != nil {
				logger.Error(errors.Wrap(err, "can not get user info"))
				continue
			}
			ai1 := &AuthorInfo{
				ID:        authorInfo.ID,
				Username:  authorInfo.Username,
				PhotoUUID: authorInfo.PhotoUUID,
				Active:    authorInfo.Active,
			}

			bodyBroadcast, err := json.Marshal(&MatchInfo{
				ID:        m.ID,
				Result:    m.Result,
				GameSlug:  m.GameSlug,
				Author1:   ai1,
				Bot1ID:    botID,
				NewScore1: diff,
				Diff1:     diff,
			})
			if err != nil {
				logger.Error(errors.Wrap(err, "can marshal match info"))
				continue
			}

			broadcast <- &BotStatusMessage{
				AuthorID: authorID,
				GameSlug: gameSlug,
				Body:     bodyBroadcast,
				Type:     "match",
			}

			body1, err := json.Marshal(&NotifyVerifyMessage{
				BotID:    botID,
				GameSlug: gameSlug,
				MatchID:  m.ID,
				Veryfied: res.Winner == 1 || res.Winner == 0,
			})
			if err != nil {
				logger.Error(errors.Wrap(err, "can not marshal body user1"))
				continue
			}

			_, err = notifyGRPC.SendNotify(context.Background(), &models.Message{
				Type: "verify",
				User: authorID,
				Game: gameSlug,
				Body: body1,
			})
			if err != nil {
				logger.Error(errors.Wrap(err, "can send notify to user1"))
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

			newStatus := "Not Verifyed. Error!\n"
			err = Bots.SetBotVerifiedByID(botID, false)
			if err != nil {
				logger.Error(errors.Wrap(err, "can update bot active status"))
				continue
			}

			m := &MatchModel{
				Result:   3, // код: ошибка
				GameSlug: gameSlug,
				Bot1:     botID,
				Author1:  authorID,
				Diff1:    0,
				Error:    sql.NullString{String: res.Error, Valid: true},
			}
			err = Matches.Create(m)
			if err != nil {
				logger.Error(errors.Wrap(err, "can not save match"))
				continue
			}

			body1, err := json.Marshal(&NotifyVerifyMessage{
				BotID:    botID,
				GameSlug: gameSlug,
				MatchID:  m.ID,
				Veryfied: false,
			})
			if err != nil {
				logger.Error(errors.Wrap(err, "can not marshal body user1"))
				continue
			}

			_, err = notifyGRPC.SendNotify(context.Background(), &models.Message{
				Type: "verify",
				User: authorID,
				Game: gameSlug,
				Body: body1,
			})
			if err != nil {
				logger.Error(errors.Wrap(err, "can send notify to user1"))
				continue
			}

			status = newStatus
		default:
			logger.Error(errors.New("can not process unknown status type"))
		}

		logger.Infof("Processing [%s]: new status: %s", event.Type, status)
	}
}
