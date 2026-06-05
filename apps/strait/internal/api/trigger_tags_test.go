package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestHandleTriggerJob_RejectsOversizedTags(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:          id,
				ProjectID:   "proj-1",
				Enabled:     true,
				TimeoutSecs: 60,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	var body strings.Builder
	body.WriteString(`{"tags":{`)
	for i := range 21 {
		if i > 0 {
			body.WriteByte(',')
		}
		body.WriteString(`"k`)
		body.WriteByte(byte('a' + i))
		body.WriteString(`":"v"`)
	}
	body.WriteString(`}}`)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/jobs/job-1/trigger", body.String(), "proj-1"))
	require.Equal(t, http.StatusBadRequest,
		w.Code,
	)
	require.True(
		t, strings.Contains(w.Body.String(), "too many tags"))

}
