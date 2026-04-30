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

// Plan 048 phase 7: ListDocuments LEFT JOINs sessions to surface
// ClassID and SessionStatus on each document so the My Work UI can
// construct navigation targets ("Open in class" / "Open live session"
// / non-clickable for dangling session_id).
func TestDocumentStore_ListDocuments_IncludesNavMetadata(t *testing.T) {
	db := testDB(t)
	documents := NewDocumentStore(db)
	users := NewUserStore(db)
	sessions := NewSessionStore(db)
	ctx := context.Background()

	user := createTestUser(t, db, users, t.Name()+"-doc-owner")
	classID, teacherID := setupSessionTest(t, db, t.Name()+"-cls")

	// Live session for the document below.
	liveSess, err := sessions.CreateSession(ctx, CreateSessionInput{
		ClassID:   strPtr(classID),
		TeacherID: teacherID,
		Title:     "Nav meta live",
	})
	require.NoError(t, err)

	// Document attached to the live session.
	liveDocID := uuid.New().String()
	_, err = db.ExecContext(ctx,
		`INSERT INTO documents (id, owner_id, session_id, language, plain_text, created_at, updated_at)
		 VALUES ($1, $2, $3, 'python', '', now(), now())`,
		liveDocID, user.ID, liveSess.ID,
	)
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM documents WHERE id = $1", liveDocID) })

	// Document with no session — class/session should be null.
	standaloneDocID := uuid.New().String()
	_, err = db.ExecContext(ctx,
		`INSERT INTO documents (id, owner_id, language, plain_text, created_at, updated_at)
		 VALUES ($1, $2, 'python', '', now(), now())`,
		standaloneDocID, user.ID,
	)
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM documents WHERE id = $1", standaloneDocID) })

	// Document with a dangling session_id (FK isn't enforced — sessions
	// row hard-deleted later). Both classId and sessionStatus should
	// resolve to null. The My Work UI treats this as non-clickable.
	danglingDocID := uuid.New().String()
	danglingSessionID := uuid.New().String()
	_, err = db.ExecContext(ctx,
		`INSERT INTO documents (id, owner_id, session_id, language, plain_text, created_at, updated_at)
		 VALUES ($1, $2, $3, 'python', '', now(), now())`,
		danglingDocID, user.ID, danglingSessionID,
	)
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM documents WHERE id = $1", danglingDocID) })

	docs, err := documents.ListDocuments(ctx, DocumentFilters{OwnerID: user.ID})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(docs), 3)

	byID := map[string]Document{}
	for _, d := range docs {
		byID[d.ID] = d
	}

	// Live-session doc: classId resolved, sessionStatus="live".
	live := byID[liveDocID]
	require.NotNil(t, live.SessionID)
	require.NotNil(t, live.ClassID, "live-session doc must surface its class id")
	assert.Equal(t, classID, *live.ClassID)
	require.NotNil(t, live.SessionStatus)
	assert.Equal(t, "live", *live.SessionStatus)

	// Standalone doc: both null.
	standalone := byID[standaloneDocID]
	assert.Nil(t, standalone.SessionID)
	assert.Nil(t, standalone.ClassID)
	assert.Nil(t, standalone.SessionStatus)

	// Dangling session_id doc: SessionID set on the document, but the
	// LEFT JOIN returns null for both ClassID and SessionStatus because
	// the sessions row doesn't exist.
	dangling := byID[danglingDocID]
	require.NotNil(t, dangling.SessionID)
	assert.Equal(t, danglingSessionID, *dangling.SessionID)
	assert.Nil(t, dangling.ClassID, "dangling session_id must yield null ClassID")
	assert.Nil(t, dangling.SessionStatus, "dangling session_id must yield null SessionStatus")
}
