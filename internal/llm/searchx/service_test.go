package searchx_test

import (
	"context"
	"errors"
	"testing"

	"mangoduck/internal/llm/responses"
	"mangoduck/internal/llm/searchx"

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

func TestSearchBuildsXSearchRequest(t *testing.T) {
	var client stubClient
	client.response = &responses.Response{
		ID:    "resp_789",
		Model: "grok-4-1-fast-reasoning",
		Output: []*responses.OutputItem{
			{
				Type: "message",
				Content: []*responses.OutputText{
					{
						Type: "output_text",
						Text: "Summary with citations",
					},
				},
			},
		},
	}

	service, err := searchx.NewService(&client, "")
	require.NoError(t, err)

	result, err := service.Search(context.Background(), &searchx.SearchRequest{
		Query:                    "latest xAI announcements",
		AllowedXHandles:          []string{"xai", "grok"},
		ExcludedXHandles:         []string{"spam"},
		FromDate:                 "2026-03-01",
		ToDate:                   "2026-03-28",
		EnableImageUnderstanding: true,
	})
	require.NoError(t, err)

	require.Equal(t, "resp_789", result.ResponseID)
	require.Equal(t, "Summary with citations", result.Text)
	require.NotNil(t, client.request)
	require.Equal(t, searchx.DefaultModel, client.request.Model)
	require.Equal(t, "required", client.request.ToolChoice)
	require.Len(t, client.request.Tools, 1)
	require.Equal(t, "x_search", client.request.Tools[0].Type)
	require.Equal(t, []string{"xai", "grok"}, client.request.Tools[0].AllowedXHandles)
	require.Equal(t, []string{"spam"}, client.request.Tools[0].ExcludedXHandles)
	require.Equal(t, "2026-03-01", client.request.Tools[0].FromDate)
	require.Equal(t, "2026-03-28", client.request.Tools[0].ToDate)
	require.True(t, client.request.Tools[0].EnableImageUnderstanding)
	require.Contains(t, client.request.Input, "latest xAI announcements")
}

func TestSearchRequiresQuery(t *testing.T) {
	var client stubClient

	service, err := searchx.NewService(&client, "")
	require.NoError(t, err)

	result, err := service.Search(context.Background(), &searchx.SearchRequest{})
	require.Nil(t, result)
	require.ErrorIs(t, err, searchx.ErrMissingQuery)
}

func TestSearchValidatesDates(t *testing.T) {
	var client stubClient

	service, err := searchx.NewService(&client, "")
	require.NoError(t, err)

	result, err := service.Search(context.Background(), &searchx.SearchRequest{
		Query:    "hello",
		FromDate: "March 1, 2026",
	})
	require.Nil(t, result)
	require.ErrorContains(t, err, "from_date")
}

func TestSearchWrapsProviderError(t *testing.T) {
	var client stubClient
	client.err = errors.New("boom")

	service, err := searchx.NewService(&client, "")
	require.NoError(t, err)

	result, err := service.Search(context.Background(), &searchx.SearchRequest{
		Query: "hello",
	})
	require.Nil(t, result)
	require.ErrorContains(t, err, "search x create response")
}
