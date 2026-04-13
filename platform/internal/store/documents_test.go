package store

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDocumentStore_GetAndList(t *testing.T) {
	db := testDB(t)
	documents := NewDocumentStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	user := createTestUser(t, db, users, t.Name())

	// Insert a document directly (documents are created by Hocuspocus, not via API)
	docID := uuid.New().String()
	_, err := db.ExecContext(ctx,
		`INSERT INTO documents (id, owner_id, language, plain_text, created_at, updated_at)
		 VALUES ($1, $2, 'python', 'x = 1', now(), now())`,
		docID, user.ID)
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM documents WHERE id = $1", docID) })

	// Get
	doc, err := documents.GetDocument(ctx, docID)
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.Equal(t, user.ID, doc.OwnerID)
	assert.Equal(t, "python", doc.Language)
	assert.Equal(t, "x = 1", doc.PlainText)

	// List by owner
	docs, err := documents.ListDocuments(ctx, DocumentFilters{OwnerID: user.ID})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(docs), 1)
}

func TestDocumentStore_GetNotFound(t *testing.T) {
	db := testDB(t)
	documents := NewDocumentStore(db)

	doc, err := documents.GetDocument(context.Background(), "00000000-0000-0000-0000-000000000000")
	assert.NoError(t, err)
	assert.Nil(t, doc)
}

func TestDocumentStore_ListNoFilters(t *testing.T) {
	db := testDB(t)
	documents := NewDocumentStore(db)

	docs, err := documents.ListDocuments(context.Background(), DocumentFilters{})
	require.NoError(t, err)
	assert.NotNil(t, docs)
	assert.Len(t, docs, 0)
}
