package server

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteProblem_DoesNotLeakRawError(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	rec := httptest.NewRecorder()
	writeProblem(rec, logger, problemArgs{
		Status:    500,
		Type:      "list-sandboxes",
		Detail:    "could not load sandboxes",
		LogReason: "internal api server error: cluster=prod-1 token=abc",
	})

	require.Equal(t, 500, rec.Code)
	require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "/errors/list-sandboxes", body["type"])
	require.Equal(t, "could not load sandboxes", body["detail"])
	require.NotContains(t, rec.Body.String(), "token=abc")
	require.Contains(t, logBuf.String(), "token=abc")
}
