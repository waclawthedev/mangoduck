package responses_test

import (
	"testing"

	"mangoduck/internal/llm/responses"

	"github.com/stretchr/testify/require"
)

func TestCreateResponseRequestValidate(t *testing.T) {
	t.Parallel()

	t.Run("missing request", func(t *testing.T) {
		t.Parallel()

		var request *responses.CreateResponseRequest

		err := request.Validate()
		require.ErrorIs(t, err, responses.ErrMissingInput)
	})

	t.Run("missing model", func(t *testing.T) {
		t.Parallel()

		var request responses.CreateResponseRequest
		request.Input = "hello"

		err := request.Validate()
		require.ErrorIs(t, err, responses.ErrMissingModel)
	})

	t.Run("missing input", func(t *testing.T) {
		t.Parallel()

		var request responses.CreateResponseRequest
		request.Model = "gpt-5-mini"

		err := request.Validate()
		require.ErrorIs(t, err, responses.ErrMissingInput)
	})

	t.Run("valid request", func(t *testing.T) {
		t.Parallel()

		var request responses.CreateResponseRequest
		request.Model = "gpt-5-mini"
		request.Input = "hello"

		err := request.Validate()
		require.NoError(t, err)
	})
}

func TestResponseOutputText(t *testing.T) {
	t.Parallel()

	var response responses.Response
	response.Output = []*responses.OutputItem{
		{
			Type: "message",
			Content: []*responses.OutputText{
				{Type: "output_text", Text: "Hello"},
				{Type: "output_text", Text: ", world"},
			},
		},
		{
			Type: "output_text",
			Text: "!",
		},
	}

	require.Equal(t, "Hello, world!", response.OutputText())
}

func TestResponseFunctionCalls(t *testing.T) {
	t.Parallel()

	var response responses.Response
	response.Output = []*responses.OutputItem{
		{
			Type:      "function_call",
			CallID:    "call_123",
			Name:      "x-search",
			Arguments: `{"query":"hello"}`,
		},
	}

	calls := response.FunctionCalls()
	require.Len(t, calls, 1)
	require.Equal(t, "call_123", calls[0].CallID)
	require.Equal(t, "x-search", calls[0].Name)
	require.JSONEq(t, `{"query":"hello"}`, calls[0].Arguments)
}

func TestAPIError(t *testing.T) {
	t.Parallel()

	var apiErr responses.APIError
	apiErr.Provider = "portkey/openai"
	apiErr.StatusCode = 401
	apiErr.Message = "invalid api key"

	require.Equal(t, "portkey/openai responses api error (status 401): invalid api key", apiErr.Error())
	var actual *responses.APIError
	require.ErrorAs(t, &apiErr, &actual)
}
