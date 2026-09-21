package repository

import (
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestGroupToolMappingDoesNotGuessPlatform(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := newGroupRepositoryWithSQL(nil, db)
	mock.ExpectQuery("SELECT group_id, tool_ids, version, effective_at, updated_by FROM group_tool_mappings").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"group_id", "tool_ids", "version", "effective_at", "updated_by"}))
	mappings, err := repo.ReadGroupToolMappings(context.Background(), []int64{7})
	require.NoError(t, err)
	require.Empty(t, mappings)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupToolMappingAtomicallyVersionsAndAudits(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := newGroupRepositoryWithSQL(nil, db)
	now := time.Now().UTC()
	mock.ExpectQuery(`(?s)WITH changed AS.*ON CONFLICT.*version=group_tool_mappings.version\+1.*INSERT INTO group_tool_mapping_audit.*SELECT group_id,tool_ids,version,effective_at,updated_by FROM changed`).WithArgs(int64(7), `["claude","codex"]`, int64(42)).WillReturnRows(sqlmock.NewRows([]string{"group_id", "tool_ids", "version", "effective_at", "updated_by"}).AddRow(7, `["claude","codex"]`, 2, now, 42))
	m, err := repo.ReplaceGroupToolMapping(context.Background(), 7, []string{"codex", "claude"}, 42)
	require.NoError(t, err)
	require.Equal(t, int64(2), m.Version)
	require.Equal(t, []string{"claude", "codex"}, m.ToolIDs)
	require.Equal(t, int64(42), m.UpdatedBy)
	require.NoError(t, mock.ExpectationsWereMet())
}
