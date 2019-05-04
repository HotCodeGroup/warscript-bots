package main

import (
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
	AuthorInfo
	ID         int64  `json:"id"`
	GameSlug   string `json:"game_slug"`
	IsActive   bool   `json:"is_active"`
	IsVerified bool   `json:"is_verified"`
}

type BotFull struct {
	Bot
	Code     string `json:"code"`
	Language Lang   `json:"lang"`
}

type BotVerifyStatusMessage struct {
	BotID     int64  `json:"bot_id"`
	AuthorID  int64  `json:"author_id"`
	GameSlug  string `json:"game_slug"`
	NewStatus string `json:"new_status"`
}
