package main

// GetMatch gets match full info by ID
// func GetMatch(w http.ResponseWriter, r *http.Request) {
// 	logger := utils.GetLogger(r, logger, "GetMatch")
// 	errWriter := utils.NewErrorResponseWriter(w, logger)
// 	vars := mux.Vars(r)

// 	matchID, err := strconv.ParseInt(vars["match_id"], 10, 64)
// 	if err != nil {
// 		errWriter.WriteError(http.StatusNotFound, errors.Wrap(err, "wrong format match_id"))
// 		return
// 	}

// 	matchInfo, err := Matches.GetMatchByID(matchID)
// 	if err != nil {
// 		if errors.Cause(err) == utils.ErrNotExists {
// 			errWriter.WriteWarn(http.StatusNotFound, errors.Wrap(err, "match not exists"))
// 		} else {
// 			errWriter.WriteError(http.StatusInternalServerError, errors.Wrap(err, "get match method error"))
// 		}
// 		return
// 	}

// 	// utils.WriteApplicationJSON(w, http.StatusOK, infoUser)
// }
