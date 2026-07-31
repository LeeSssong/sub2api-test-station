package repository

import (
	"context"
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestVerifyMigrationsAcceptsMatchingRowsWithoutWrites(t *testing.T) {
	fsys := fstest.MapFS{
		"001_init.sql":     &fstest.MapFile{Data: []byte("CREATE TABLE users (id INT);\n")},
		"002_add_name.sql": &fstest.MapFile{Data: []byte("ALTER TABLE users ADD COLUMN name TEXT;\n")},
	}

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectQuery("SELECT filename, checksum FROM schema_migrations").
		WillReturnRows(sqlmock.NewRows([]string{"filename", "checksum"}).
			AddRow("001_init.sql", "5ea918fac5561634f4b577815b41483e5882b9c57dd3bd2351e3422d641af545").
			AddRow("002_add_name.sql", "60172bfbea04b7d899548cee12db965817115948c8cbda930531c0abfe9c2d20"))
	err = verifyMigrationsFS(context.Background(), db, fsys)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestVerifyMigrationsReportsMissingTableWithoutWrites(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectQuery("SELECT filename, checksum FROM schema_migrations").
		WillReturnError(errors.New(`pq: relation "schema_migrations" does not exist`))

	err = verifyMigrationsFS(context.Background(), db, fstest.MapFS{
		"001_init.sql": &fstest.MapFile{Data: []byte("CREATE TABLE users (id INT);")},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "schema_migrations")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestVerifyMigrationsReportsMissingMigrationRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectQuery("SELECT filename, checksum FROM schema_migrations").
		WillReturnRows(sqlmock.NewRows([]string{"filename", "checksum"}))

	err = verifyMigrationsFS(context.Background(), db, fstest.MapFS{
		"001_init.sql": &fstest.MapFile{Data: []byte("CREATE TABLE users (id INT);")},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "001_init.sql")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestVerifyMigrationsReportsChecksumMismatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectQuery("SELECT filename, checksum FROM schema_migrations").
		WillReturnRows(sqlmock.NewRows([]string{"filename", "checksum"}).
			AddRow("001_init.sql", "db-checksum"))

	err = verifyMigrationsFS(context.Background(), db, fstest.MapFS{
		"001_init.sql": &fstest.MapFile{Data: []byte("CREATE TABLE users (id INT);")},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "db=db-checksum")
	require.Contains(t, err.Error(), "file=5ea918fac5561634f4b577815b41483e5882b9c57dd3bd2351e3422d641af545")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestVerifyMigrationsAcceptsKnownCompatibilityChecksum(t *testing.T) {
	content, err := fs.ReadFile(migrations.FS, "054_drop_legacy_cache_columns.sql")
	require.NoError(t, err)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectQuery("SELECT filename, checksum FROM schema_migrations").
		WillReturnRows(sqlmock.NewRows([]string{"filename", "checksum"}).
			AddRow("054_drop_legacy_cache_columns.sql", "182c193f3359946cf094090cd9e57d5c3fd9abaffbc1e8fc378646b8a6fa12b4"))

	err = verifyMigrationsFS(context.Background(), db, fstest.MapFS{
		"054_drop_legacy_cache_columns.sql": &fstest.MapFile{Data: content},
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMigrationSetHashIsStableAndContentSensitive(t *testing.T) {
	first := fstest.MapFS{
		"002_b.sql":   &fstest.MapFile{Data: []byte(" B \n")},
		"001_a.sql":   &fstest.MapFile{Data: []byte(" A \n")},
		"ignored.txt": &fstest.MapFile{Data: []byte("ignored")},
	}
	second := fstest.MapFS{
		"001_a.sql": &fstest.MapFile{Data: []byte(" A \n")},
		"002_b.sql": &fstest.MapFile{Data: []byte(" B \n")},
	}
	changed := fstest.MapFS{
		"001_a.sql": &fstest.MapFile{Data: []byte(" A changed \n")},
		"002_b.sql": &fstest.MapFile{Data: []byte(" B \n")},
	}

	hash1, err := MigrationSetHash(first)
	require.NoError(t, err)
	hash2, err := MigrationSetHash(second)
	require.NoError(t, err)
	hash3, err := MigrationSetHash(changed)
	require.NoError(t, err)
	require.Len(t, hash1, 64)
	require.Equal(t, "c7a1ee4aa508d48c88de7f8d58de4365b639d17e466097ff47b40a6b45cc6904", hash1)
	require.Equal(t, hash1, hash2)
	require.NotEqual(t, hash1, hash3)
}
