package main

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/streadway/amqp"

	"github.com/sirupsen/logrus"
)

const (
	testerQueueName = "tester_rpc_queue"
)

type TesterStatusQueue struct {
	Type string          `json:"type"`
	Body json.RawMessage `json:"body"`
}

type TesterStatusUpdate struct {
	NewStatus string `json:"new_status"`
}

type TesterStatusError struct {
	Error string `json:"error"`
}

type TesterStatusResult struct {
	Winner int             `json:"result"`
	States json.RawMessage `json:"states"`
}

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
			upd := &TesterStatusUpdate{}
			err := json.Unmarshal(event.Body, upd)
			if err != nil {
				logger.Error(errors.Wrap(err, "can not unmarshal update status body"))
				continue
			}

			body, _ := json.Marshal(&BotStatus{
				BotID:     botID,
				NewStatus: upd.NewStatus,
			})

			broadcast <- &BotStatusMessage{
				AuthorID: authorID,
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

			body, _ := json.Marshal(&BotStatus{
				BotID:     botID,
				NewStatus: newStatus,
			})

			broadcast <- &BotStatusMessage{
				AuthorID: authorID,
				GameSlug: gameSlug,
				Body:     body,
			}

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

			body, _ := json.Marshal(&BotStatus{
				BotID:     botID,
				NewStatus: newStatus,
			})

			broadcast <- &BotStatusMessage{
				AuthorID: authorID,
				GameSlug: gameSlug,
				Body:     body,
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
