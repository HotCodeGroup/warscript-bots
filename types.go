package main

import (
	"encoding/json"

	"github.com/HotCodeGroup/warscript-utils/utils"
)

type Lang string

type BotUpload struct {
	Code     string `json:"code"`
	GameSlug string `json:"game_slug"`
	Language Lang   `json:"lang"`
}

var availableLanguages = map[Lang]struct{}{
	// JS - JavaScript
	"JS": {},
}

func (bu *BotUpload) Validate() error {
	if _, ok := availableLanguages[bu.Language]; !ok {
		return &utils.ValidationError{
			"lang": utils.ErrInvalid.Error(),
		}
	}

	return nil
}

type AuthorInfo struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	PhotoUUID string `json:"photo_uuid"`
	Active    bool   `json:"active"`
}

type Bot struct {
	Author     *AuthorInfo `json:"author"`
	ID         int64       `json:"id"`
	GameSlug   string      `json:"game_slug"`
	IsActive   bool        `json:"is_active"`
	IsVerified bool        `json:"is_verified"`
	Score      int64       `json:"score"`
}

type BotFull struct {
	Bot
	Code     string `json:"code"`
	Language Lang   `json:"lang"`
}

type BotStatusMessage struct {
	AuthorID int64           `json:"-"`
	GameSlug string          `json:"-"`
	Type     string          `json:"type"`
	Body     json.RawMessage `json:"body"`
}

type BotStatus struct {
	BotID     int64  `json:"bot_id"`
	NewStatus string `json:"new_status"`
}

type MatchStatus struct {
	Bot1ID    int64  `json:"bot1_id"`
	Bot2ID    int64  `json:"bot2_id"`
	Author1ID int64  `json:"author1_id"`
	Author2ID int64  `json:"author2_id"`
	NewStatus string `json:"new_status"`
}
