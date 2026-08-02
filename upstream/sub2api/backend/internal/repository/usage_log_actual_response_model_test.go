package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestUpdateActualResponseModelByRequestID(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &usageLogRepository{sql: db}
	mock.ExpectExec(`UPDATE usage_logs SET actual_response_model = \$1 WHERE request_id = \$2`).
		WithArgs("gpt-5.6-terra", "req-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.UpdateActualResponseModelByRequestID(context.Background(), " req-1 ", " gpt-5.6-terra "))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateActualResponseModelByRequestIDIgnoresEmptyModel(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &usageLogRepository{sql: db}
	require.NoError(t, repo.UpdateActualResponseModelByRequestID(context.Background(), "req-1", "   "))
	require.NoError(t, mock.ExpectationsWereMet())
}
