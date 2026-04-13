package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnnotationStore_CreateAndList(t *testing.T) {
	db := testDB(t)
	annotations := NewAnnotationStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	user := createTestUser(t, db, users, t.Name())

	a, err := annotations.CreateAnnotation(ctx, CreateAnnotationInput{
		DocumentID: "test-doc-" + t.Name(),
		AuthorID:   user.ID,
		AuthorType: "teacher",
		LineStart:  "5",
		LineEnd:    "10",
		Content:    "Consider renaming this variable",
	})
	require.NoError(t, err)
	require.NotNil(t, a)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM code_annotations WHERE id = $1", a.ID) })

	assert.Equal(t, "5", a.LineStart)
	assert.Equal(t, "10", a.LineEnd)
	assert.Nil(t, a.Resolved)

	list, err := annotations.ListAnnotations(ctx, "test-doc-"+t.Name())
	require.NoError(t, err)
	assert.Len(t, list, 1)
}

func TestAnnotationStore_Resolve(t *testing.T) {
	db := testDB(t)
	annotations := NewAnnotationStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	user := createTestUser(t, db, users, t.Name())

	a, err := annotations.CreateAnnotation(ctx, CreateAnnotationInput{
		DocumentID: "resolve-doc-" + t.Name(),
		AuthorID:   user.ID, AuthorType: "teacher",
		LineStart: "1", LineEnd: "1", Content: "Fix this",
	})
	require.NoError(t, err)
	t.Cleanup(func() { db.ExecContext(ctx, "DELETE FROM code_annotations WHERE id = $1", a.ID) })

	resolved, err := annotations.ResolveAnnotation(ctx, a.ID)
	require.NoError(t, err)
	require.NotNil(t, resolved)
	assert.NotNil(t, resolved.Resolved)
}

func TestAnnotationStore_Delete(t *testing.T) {
	db := testDB(t)
	annotations := NewAnnotationStore(db)
	users := NewUserStore(db)
	ctx := context.Background()

	user := createTestUser(t, db, users, t.Name())

	a, err := annotations.CreateAnnotation(ctx, CreateAnnotationInput{
		DocumentID: "del-doc-" + t.Name(),
		AuthorID:   user.ID, AuthorType: "ai",
		LineStart: "1", LineEnd: "3", Content: "AI suggestion",
	})
	require.NoError(t, err)

	deleted, err := annotations.DeleteAnnotation(ctx, a.ID)
	require.NoError(t, err)
	require.NotNil(t, deleted)

	list, err := annotations.ListAnnotations(ctx, "del-doc-"+t.Name())
	require.NoError(t, err)
	assert.Len(t, list, 0)
}

func TestAnnotationStore_Delete_NotFound(t *testing.T) {
	db := testDB(t)
	annotations := NewAnnotationStore(db)

	deleted, err := annotations.DeleteAnnotation(context.Background(), "00000000-0000-0000-0000-000000000000")
	assert.NoError(t, err)
	assert.Nil(t, deleted)
}
