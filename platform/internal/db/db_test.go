package db

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpen_Success(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set -- skipping integration test")
	}

	db, err := Open(url)
	require.NoError(t, err)
	require.NotNil(t, db)
	defer db.Close()

	// Verify connection works
	var result int
	err = db.QueryRow("SELECT 1").Scan(&result)
	require.NoError(t, err)
	assert.Equal(t, 1, result)
}

func TestOpen_InvalidURL(t *testing.T) {
	_, err := Open("postgresql://invalid:invalid@127.0.0.1:59999/nonexistent")
	assert.Error(t, err)
}

func TestOpen_EmptyURL(t *testing.T) {
	_, err := Open("")
	assert.Error(t, err)
}
