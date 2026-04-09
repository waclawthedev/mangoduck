package websearch_test

import (
	"context"
	"errors"
	"testing"

	"mangoduck/internal/llm/responses"
	"mangoduck/internal/llm/websearch"

	"github.com/stretchr/testify/require"
)

type stubClient struct {
	request  *responses.CreateResponseRequest
	response *responses.Response
	err      error
}

func (c *stubClient) CreateResponse(_ context.Context, request *responses.CreateResponseRequest) (*responses.Response, error) {
	c.request = request

	return c.response, c.err
}

func TestSearchBuildsWebSearchRequest(t *testing.T) {
	var client stubClient
	client.response = &responses.Response{
		ID:    "resp_999",
		Model: "gpt-5.4-nano",
		Output: []*responses.OutputItem{
			{
				Type: "message",
				Content: []*responses.OutputText{
					{
						Type: "output_text",
						Text: "Web summary with citations",
					},
				},
			},
		},
	}

	service, err := websearch.NewService(&client, "")
	require.NoError(t, err)

	result, err := service.Search(context.Background(), &websearch.SearchRequest{
		Query:                    "latest xAI announcements on the web",
		AllowedDomains:           []string{"x.ai", "blog.x.ai"},
		EnableImageUnderstanding: true,
	})
	require.NoError(t, err)

	require.Equal(t, "resp_999", result.ResponseID)
	require.Equal(t, "Web summary with citations", result.Text)
	require.NotNil(t, client.request)
	require.Equal(t, websearch.DefaultModel, client.request.Model)
	require.Equal(t, "required", client.request.ToolChoice)
	require.Len(t, client.request.Tools, 1)
	require.Equal(t, "web_search", client.request.Tools[0].Type)
	require.NotNil(t, client.request.Tools[0].Filters)
	require.Equal(t, []string{"x.ai", "blog.x.ai"}, client.request.Tools[0].Filters.AllowedDomains)
	require.Nil(t, client.request.Tools[0].Filters.ExcludedDomains)
	require.True(t, client.request.Tools[0].EnableImageUnderstanding)
	require.Contains(t, client.request.Input, "latest xAI announcements on the web")
}

func TestSearchRequiresQuery(t *testing.T) {
	var client stubClient

	service, err := websearch.NewService(&client, "")
	require.NoError(t, err)

	result, err := service.Search(context.Background(), &websearch.SearchRequest{})
	require.Nil(t, result)
	require.ErrorIs(t, err, websearch.ErrMissingQuery)
}

func TestSearchRejectsConflictingDomainFilters(t *testing.T) {
	var client stubClient

	service, err := websearch.NewService(&client, "")
	require.NoError(t, err)

	result, err := service.Search(context.Background(), &websearch.SearchRequest{
		Query:           "hello",
		AllowedDomains:  []string{"x.ai"},
		ExcludedDomains: []string{"example.com"},
	})
	require.Nil(t, result)
	require.ErrorIs(t, err, websearch.ErrConflictingDomainFilter)
}

func TestSearchWrapsProviderError(t *testing.T) {
	var client stubClient
	client.err = errors.New("boom")

	service, err := websearch.NewService(&client, "")
	require.NoError(t, err)

	result, err := service.Search(context.Background(), &websearch.SearchRequest{
		Query: "hello",
	})
	require.Nil(t, result)
	require.ErrorContains(t, err, "web search create response")
}
