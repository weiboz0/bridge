package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupBookEnv(t *testing.T, suffix string) (*BookStore, *ChapterStore, string, string) {
	t.Helper()
	db := testDB(t)
	ctx := context.Background()
	orgs := NewOrgStore(db)
	users := NewUserStore(db)
	books := NewBookStore(db)
	chapters := NewChapterStore(db)
	org := createTestOrg(t, db, orgs, suffix)
	user := createTestUser(t, db, users, suffix)
	t.Cleanup(func() {
		db.ExecContext(ctx, "DELETE FROM chapters WHERE created_by = $1", user.ID)
		db.ExecContext(ctx, "DELETE FROM books WHERE created_by = $1", user.ID)
	})
	return books, chapters, org.ID, user.ID
}

func TestBookStore_CreatePlatform(t *testing.T) {
	books, _, _, userID := setupBookEnv(t, t.Name())
	book, err := books.CreateBook(context.Background(), CreateBookInput{Title: "  Platform Book  ", Scope: "platform", CreatedBy: userID})
	require.NoError(t, err)
	require.NotNil(t, book)
	assert.Equal(t, "Platform Book", book.Title)
	assert.Equal(t, "platform", book.Scope)
	assert.Nil(t, book.ScopeID)
}

func TestBookStore_CreateOrg(t *testing.T) {
	books, _, orgID, userID := setupBookEnv(t, t.Name())
	book, err := books.CreateBook(context.Background(), CreateBookInput{Title: "Org Book", Scope: "org", ScopeID: &orgID, CreatedBy: userID})
	require.NoError(t, err)
	require.NotNil(t, book.ScopeID)
	assert.Equal(t, orgID, *book.ScopeID)
}

func TestBookStore_CreateValidation(t *testing.T) {
	books, _, orgID, userID := setupBookEnv(t, t.Name())
	cases := []CreateBookInput{
		{Title: "", Scope: "platform", CreatedBy: userID},
		{Title: "Book", Scope: "personal", CreatedBy: userID},
		{Title: "Book", Scope: "platform", ScopeID: &orgID, CreatedBy: userID},
		{Title: "Book", Scope: "org", CreatedBy: userID},
	}
	for _, tc := range cases {
		_, err := books.CreateBook(context.Background(), tc)
		require.Error(t, err)
	}
}

func TestBookStore_GetAndNotFound(t *testing.T) {
	books, _, _, userID := setupBookEnv(t, t.Name())
	book, err := books.CreateBook(context.Background(), CreateBookInput{Title: "Get Book", Scope: "platform", CreatedBy: userID})
	require.NoError(t, err)
	got, err := books.GetBook(context.Background(), book.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, book.ID, got.ID)
	missing, err := books.GetBook(context.Background(), "00000000-0000-0000-0000-000000000000")
	require.NoError(t, err)
	assert.Nil(t, missing)
}

func TestBookStore_ListFiltered(t *testing.T) {
	books, _, orgID, userID := setupBookEnv(t, t.Name())
	_, err := books.CreateBook(context.Background(), CreateBookInput{Title: "Platform", Scope: "platform", CreatedBy: userID})
	require.NoError(t, err)
	orgBook, err := books.CreateBook(context.Background(), CreateBookInput{Title: "Org", Scope: "org", ScopeID: &orgID, CreatedBy: userID})
	require.NoError(t, err)
	got, err := books.ListBooks(context.Background(), BookFilter{Scope: "org", ScopeID: &orgID})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, orgBook.ID, got[0].ID)
}

func TestBookStore_Update(t *testing.T) {
	books, _, _, userID := setupBookEnv(t, t.Name())
	book, err := books.CreateBook(context.Background(), CreateBookInput{Title: "Old", Scope: "platform", CreatedBy: userID})
	require.NoError(t, err)
	title := "New"
	description := "Updated description"
	updated, err := books.UpdateBook(context.Background(), book.ID, UpdateBookInput{Title: &title, Description: &description})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "New", updated.Title)
	assert.Equal(t, description, updated.Description)
}

func TestBookStore_Delete(t *testing.T) {
	books, _, _, userID := setupBookEnv(t, t.Name())
	book, err := books.CreateBook(context.Background(), CreateBookInput{Title: "Delete", Scope: "platform", CreatedBy: userID})
	require.NoError(t, err)
	require.NoError(t, books.DeleteBook(context.Background(), book.ID))
	err = books.DeleteBook(context.Background(), book.ID)
	require.ErrorIs(t, err, ErrBookNotFound)
}

func TestBookStore_DeleteSetsChapterBookIDNull(t *testing.T) {
	books, chapters, _, userID := setupBookEnv(t, t.Name())
	book, err := books.CreateBook(context.Background(), CreateBookInput{Title: "Cascade", Scope: "platform", CreatedBy: userID})
	require.NoError(t, err)
	chapter, err := chapters.CreateChapter(context.Background(), CreateChapterInput{
		Scope:     "platform",
		Title:     "Chapter",
		CreatedBy: userID,
		BookID:    &book.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, chapter.BookID)
	require.NoError(t, books.DeleteBook(context.Background(), book.ID))
	got, err := chapters.GetChapter(context.Background(), chapter.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Nil(t, got.BookID)
}
