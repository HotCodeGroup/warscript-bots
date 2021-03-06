package main

var h *hub

type hub struct {
	// UserID -> GameID -> SessionID -> byte channel
	sessions map[int64]map[string]map[string]chan *BotStatusMessage

	broadcast  chan *BotStatusMessage
	register   chan *BotVerifyClient
	unregister chan *BotVerifyClient
}

func (h *hub) registerClient(client *BotVerifyClient) {
	if _, ok := h.sessions[client.UserID]; !ok {
		h.sessions[client.UserID] = make(map[string]map[string]chan *BotStatusMessage)
	}

	if _, ok := h.sessions[client.UserID][client.GameSlug]; !ok {
		h.sessions[client.UserID][client.GameSlug] = make(map[string]chan *BotStatusMessage)
	}

	h.sessions[client.UserID][client.GameSlug][client.SessionID] = client.send
}

func (h *hub) unregisterClient(client *BotVerifyClient) {
	if _, ok := h.sessions[client.UserID]; ok {
		if _, ok := h.sessions[client.UserID][client.GameSlug]; ok {
			if _, ok := h.sessions[client.UserID][client.GameSlug][client.SessionID]; ok {
				delete(h.sessions[client.UserID][client.GameSlug], client.SessionID)
				close(client.send)
			}

			if len(h.sessions[client.UserID][client.GameSlug]) == 0 {
				delete(h.sessions[client.UserID], client.GameSlug)
			}
		}

		if len(h.sessions[client.UserID]) == 0 {
			delete(h.sessions, client.UserID)
		}
	}
}

func (h *hub) run() {
	for {
		select {
		case client := <-h.register:
			h.registerClient(client)
		case client := <-h.unregister:
			h.unregisterClient(client)
		case message := <-h.broadcast:
			if _, ok := h.sessions[message.AuthorID]; ok {
				// для тех, кто слушает какого-то одного автора + игру

				for _, send := range h.sessions[message.AuthorID][message.GameSlug] {
					send <- message
				}

				// для тех, кто слушает профиль игрока
				for _, send := range h.sessions[message.AuthorID][""] {
					send <- message
				}
			}

			if !message.Private {
				if _, ok := h.sessions[0]; ok {
					// для тех, кто слушает игру
					for _, send := range h.sessions[0][message.GameSlug] {
						send <- message
					}

					// для тех, кто слушает всё
					for _, send := range h.sessions[0][""] {
						send <- message
					}
				}
			}
		}
	}
}
