package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// problemArgs holds all inputs for a problem+json response. The Detail field
// is client-safe; LogReason is only ever written to the logger, never to the
// response body.
type problemArgs struct {
	Status    int    // HTTP status code
	Type      string // short stable type slug, used to build /errors/<type>
	Detail    string // client-safe message; MUST NOT contain raw err.Error()
	LogReason string // verbose reason logged at error level; never sent to the client
}

// writeProblem emits an RFC 7807 problem+json response. If logger is non-nil
// and LogReason is non-empty, the full reason is logged at error level before
// the sanitised response is written to the client.
func writeProblem(w http.ResponseWriter, logger *slog.Logger, p problemArgs) {
	if logger != nil && p.LogReason != "" {
		logger.Error("api_problem",
			"status", p.Status,
			"type", p.Type,
			"reason", p.LogReason,
		)
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":   "/errors/" + p.Type,
		"status": p.Status,
		"detail": p.Detail,
	})
}
