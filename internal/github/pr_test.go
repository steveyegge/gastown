package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestClient creates a Client pointing at a test HTTP server.
func newTestClient(t *testing.T, handler http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := NewClient(
		WithToken("test-token"),
		WithHTTPClient(srv.Client()),
		WithRESTBase(srv.URL),
		WithGraphQLBase(srv.URL+"/graphql"),
	)
	require.NoError(t, err)
	return c, srv
}

func TestNewClient_RequiresToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	_, err := NewClient()
	assert.ErrorContains(t, err, "GITHUB_TOKEN is required")
}

func TestNewClient_FromEnv(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "env-token")
	c, err := NewClient()
	require.NoError(t, err)
	assert.Equal(t, "env-token", c.token)
}

func TestCreateDraftPR(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /repos/octo/repo/pulls", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, true, body["draft"])
		assert.Equal(t, "feat-branch", body["head"])
		assert.Equal(t, "main", body["base"])

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"number":   42,
			"html_url": "https://github.com/octo/repo/pull/42",
		})
	})

	c, _ := newTestClient(t, mux)
	result, err := c.CreateDraftPR(t.Context(), "octo", "repo", "feat-branch", "main", "Add feature", "Description")
	require.NoError(t, err)
	assert.Equal(t, 42, result.Number)
	assert.Equal(t, "https://github.com/octo/repo/pull/42", result.URL)
}

func TestCreateDraftPR_CrossRepoFork(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /repos/upstream/repo/pulls", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "forkOwner:feat-branch", body["head"])
		assert.Equal(t, "main", body["base"])

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"number":   7,
			"html_url": "https://github.com/upstream/repo/pull/7",
		})
	})

	c, _ := newTestClient(t, mux)
	result, err := c.CreateDraftPR(t.Context(), "upstream", "repo", "forkOwner:feat-branch", "main", "Fork PR", "From fork")
	require.NoError(t, err)
	assert.Equal(t, 7, result.Number)
}

func TestUpdatePRDescription(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("PATCH /repos/octo/repo/pulls/42", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "Updated body", body["body"])
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	})

	c, _ := newTestClient(t, mux)
	err := c.UpdatePRDescription(t.Context(), "octo", "repo", 42, "Updated body")
	require.NoError(t, err)
}

func TestConvertDraftToReady(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()

	// REST call to get node ID
	mux.HandleFunc("GET /repos/octo/repo/pulls/42", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"node_id": "PR_kwDOTest",
		})
	})

	// GraphQL mutation
	mux.HandleFunc("POST /graphql", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		query := body["query"].(string)
		assert.Contains(t, query, "markPullRequestReadyForReview")

		vars := body["variables"].(map[string]any)
		assert.Equal(t, "PR_kwDOTest", vars["id"])

		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"markPullRequestReadyForReview": map[string]any{
					"pullRequest": map[string]any{"id": "PR_kwDOTest"},
				},
			},
		})
	})

	c, _ := newTestClient(t, mux)
	err := c.ConvertDraftToReady(t.Context(), "octo", "repo", 42)
	require.NoError(t, err)
}

func TestGetPRReviewStatus_Approved(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/octo/repo/pulls/42/reviews", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{"state": "COMMENTED", "user": map[string]any{"login": "alice"}},
			{"state": "APPROVED", "user": map[string]any{"login": "alice"}},
			{"state": "APPROVED", "user": map[string]any{"login": "bob"}},
		})
	})

	c, _ := newTestClient(t, mux)
	state, err := c.GetPRReviewStatus(t.Context(), "octo", "repo", 42)
	require.NoError(t, err)
	assert.Equal(t, ReviewApproved, state)
}

func TestGetPRReviewStatus_ChangesRequested(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/octo/repo/pulls/42/reviews", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{"state": "APPROVED", "user": map[string]any{"login": "alice"}},
			{"state": "CHANGES_REQUESTED", "user": map[string]any{"login": "bob"}},
		})
	})

	c, _ := newTestClient(t, mux)
	state, err := c.GetPRReviewStatus(t.Context(), "octo", "repo", 42)
	require.NoError(t, err)
	assert.Equal(t, ReviewChangesRequired, state)
}

func TestGetPRReviewStatus_NoReviews(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/octo/repo/pulls/42/reviews", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{})
	})

	c, _ := newTestClient(t, mux)
	state, err := c.GetPRReviewStatus(t.Context(), "octo", "repo", 42)
	require.NoError(t, err)
	assert.Equal(t, ReviewPending, state)
}

func TestGetPRReviewComments(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/octo/repo/pulls/42/comments", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{
				"id":         101,
				"body":       "Fix this",
				"path":       "main.go",
				"line":       10,
				"created_at": "2026-01-01T00:00:00Z",
				"html_url":   "https://github.com/octo/repo/pull/42#comment-101",
				"user":       map[string]any{"login": "alice"},
			},
		})
	})

	c, _ := newTestClient(t, mux)
	comments, err := c.GetPRReviewComments(t.Context(), "octo", "repo", 42)
	require.NoError(t, err)
	require.Len(t, comments, 1)
	assert.Equal(t, int64(101), comments[0].ID)
	assert.Equal(t, "Fix this", comments[0].Body)
	assert.Equal(t, "alice", comments[0].User)
	assert.Equal(t, "main.go", comments[0].Path)
}

func TestReplyToPRComment(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /repos/octo/repo/pulls/42/comments", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "Thanks, fixed!", body["body"])
		assert.Equal(t, float64(101), body["in_reply_to"])
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{}`))
	})

	c, _ := newTestClient(t, mux)
	err := c.ReplyToPRComment(t.Context(), "octo", "repo", 42, 101, "Thanks, fixed!")
	require.NoError(t, err)
}

func TestMergePR(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /repos/octo/repo/pulls/42/merge", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "squash", body["merge_method"])
		json.NewEncoder(w).Encode(map[string]any{"merged": true})
	})

	c, _ := newTestClient(t, mux)
	err := c.MergePR(t.Context(), "octo", "repo", 42, "squash")
	require.NoError(t, err)
}

func TestGetRepoMergeMethod_Squash(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/octo/repo", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"allow_merge_commit": true,
			"allow_squash_merge": true,
			"allow_rebase_merge": true,
		})
	})

	c, _ := newTestClient(t, mux)
	method, err := c.GetRepoMergeMethod(t.Context(), "octo", "repo")
	require.NoError(t, err)
	assert.Equal(t, "squash", method)
}

func TestGetRepoMergeMethod_RebaseOnly(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/octo/repo", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"allow_merge_commit": false,
			"allow_squash_merge": false,
			"allow_rebase_merge": true,
		})
	})

	c, _ := newTestClient(t, mux)
	method, err := c.GetRepoMergeMethod(t.Context(), "octo", "repo")
	require.NoError(t, err)
	assert.Equal(t, "rebase", method)
}

func TestGetRepoMergeMethod_NoneEnabled(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/octo/repo", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"allow_merge_commit": false,
			"allow_squash_merge": false,
			"allow_rebase_merge": false,
		})
	})

	c, _ := newTestClient(t, mux)
	_, err := c.GetRepoMergeMethod(t.Context(), "octo", "repo")
	assert.ErrorContains(t, err, "no merge methods enabled")
}

func TestAPIError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/octo/repo/pulls/999/reviews", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Not Found"}`))
	})

	c, _ := newTestClient(t, mux)
	_, err := c.GetPRReviewStatus(t.Context(), "octo", "repo", 999)
	require.Error(t, err)

	var apiErr *APIError
	assert.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 404, apiErr.StatusCode)
}

func TestConvertDraftToReady_GraphQLError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/octo/repo/pulls/42", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"node_id": "PR_kwDOTest"})
	})
	mux.HandleFunc("POST /graphql", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"errors": []map[string]any{
				{"message": "Pull request is not a draft"},
			},
		})
	})

	c, _ := newTestClient(t, mux)
	err := c.ConvertDraftToReady(context.Background(), "octo", "repo", 42)
	assert.ErrorContains(t, err, "Pull request is not a draft")
}
