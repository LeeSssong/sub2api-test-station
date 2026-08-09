package repository

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageLogRequestEventIncludesActualResponseModel(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := newUsageLogRepositoryWithSQL(nil, db)
	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)
	mock.ExpectExec("INSERT INTO externalization_outbox").WithArgs(
		sqlmock.AnyArg(), "request.completed", sqlmock.AnyArg(), "sub2api-core", 1, payloadMatcher{want: "gpt-5.6-terra"},
	).WillReturnResult(sqlmock.NewResult(0, 1))
	actual := "gpt-5.6-terra"
	log := &service.UsageLog{RequestID: "req-model", AccountID: 7, Model: "gpt-5.6-sol", RequestedModel: "gpt-5.6-sol", ActualResponseModel: &actual, InputTokens: 2, OutputTokens: 3, TotalCost: 1.25, ActualCost: .75, CreatedAt: time.Now().UTC()}
	require.NoError(t, repo.appendRequestEvent(context.Background(), tx, log))
	mock.ExpectCommit()
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())

}

func TestUsageLogCreateBestEffortRollsBackWhenAppendFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := newUsageLogRepositoryWithSQL(nil, db)
	log := &service.UsageLog{RequestID: "req-rollback", APIKeyID: 9, AccountID: 7, Model: "model", TotalCost: 1, ActualCost: 1}
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO usage_logs").WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(11, time.Now()))
	mock.ExpectExec("INSERT INTO externalization_outbox").WillReturnError(errors.New("outbox unavailable"))
	mock.ExpectRollback()
	err = repo.CreateBestEffort(context.Background(), log)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

type payloadMatcher struct{ want string }

func (m payloadMatcher) Match(v driver.Value) bool {
	b, ok := v.([]byte)
	if !ok {
		return false
	}
	var payload map[string]any
	if json.Unmarshal(b, &payload) != nil {
		return false
	}
	actual, _ := payload["actual_response_model"].(string)
	return actual == m.want
}
