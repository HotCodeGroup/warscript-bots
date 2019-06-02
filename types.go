package main

import (
	"encoding/json"
	"time"

	"github.com/HotCodeGroup/warscript-utils/utils"
)

// Lang по сути ENUM с доступными языками
type Lang string

// BotUpload структура от front для загрузки бота на сервер
type BotUpload struct {
	Code     string `json:"code"`
	GameSlug string `json:"game_slug"`
	Language Lang   `json:"lang"`
}

var availableLanguages = map[Lang]struct{}{
	// JS - JavaScript
	"JS": {},
}

// Validate проверка полей входящего бота, на соответствие требованиям
func (bu *BotUpload) Validate() error {
	if _, ok := availableLanguages[bu.Language]; !ok {
		return &utils.ValidationError{
			"lang": utils.ErrInvalid.Error(),
		}
	}

	return nil
}

// AuthorInfo информация об автора бота
type AuthorInfo struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	PhotoUUID string `json:"photo_uuid"`
	Active    bool   `json:"active"`
}

// Bot частичная информация о боте
type Bot struct {
	Author     *AuthorInfo `json:"author"`
	ID         int64       `json:"id"`
	GameSlug   string      `json:"game_slug"`
	IsActive   bool        `json:"is_active"`
	IsVerified bool        `json:"is_verified"`
	Score      int64       `json:"score"`
}

// BotFull полная информация о боте
type BotFull struct {
	Bot
	Code     string `json:"code"`
	Language Lang   `json:"lang"`
}

// BotStatusMessage обновление статуса бота, например: прошел проверку
type BotStatusMessage struct {
	Private  bool            `json:"-"`
	AuthorID int64           `json:"-"`
	GameSlug string          `json:"-"`
	Type     string          `json:"type"`
	Body     json.RawMessage `json:"body"`
}

// BotStatus новый статус бота
type BotStatus struct {
	BotID     int64  `json:"bot_id"`
	NewStatus string `json:"new_status"`
}

// MatchInfo краткая информация о матче
type MatchInfo struct {
	ID        int64       `json:"id"`
	Result    int         `json:"result"`
	GameSlug  string      `json:"game_slug"`
	Author1   *AuthorInfo `json:"author_1"`
	Author2   *AuthorInfo `json:"author_2"`
	Bot1ID    int64       `json:"bot1_id"`
	Bot2ID    int64       `json:"bot2_id"`
	NewScore1 int64       `json:"new_score1"`
	NewScore2 int64       `json:"new_score2"`
	Diff1     int64       `json:"diff1"`
	Diff2     int64       `json:"diff2"`
}

// Replay повтор матча для плеера
type Replay struct {
	Info   json.RawMessage `json:"info"`
	States json.RawMessage `json:"states"`
	Winner int             `json:"winner"`
}

// MatchFullInfo полная информация о матче
type MatchFullInfo struct {
	MatchInfo
	Replay    *Replay         `json:"replay"`
	Logs      json.RawMessage `json:"logs"`
	Error     string          `json:"error"`
	Timestamp time.Time       `json:"timestamp"`
	Code      string          `json:"code"`
}

// NotifyMatchMessage сообщение для сервиса нотификации о матче
type NotifyMatchMessage struct {
	BotID    int64  `json:"bot_id"`
	GameSlug string `json:"game_slug"`
	MatchID  int64  `json:"match_id"`
	Diff     int64  `json:"diff"`
}

// NotifyVerifyMessage сообщение для сервиса нотификации о прохождении проверки
type NotifyVerifyMessage struct {
	BotID    int64  `json:"bot_id"`
	GameSlug string `json:"game_slug"`
	MatchID  int64  `json:"match_id"`
	Veryfied bool   `json:"veryfied"`
}
