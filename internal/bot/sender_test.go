package bot

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	tele "gopkg.in/telebot.v4"
)

func TestTelegramSender_SendNormalizesEscapedTelegramHTML(t *testing.T) {
	t.Parallel()

	var (
		mu    sync.Mutex
		calls []map[string]string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, r.Body.Close())

		var payload map[string]string
		err = json.Unmarshal(body, &payload)
		require.NoError(t, err)

		mu.Lock()
		calls = append(calls, payload)
		mu.Unlock()

		_, err = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":0,"chat":{"id":7,"type":"private"},"text":"ok"}}`))
		require.NoError(t, err)
	}))
	defer server.Close()

	botAPI, err := tele.NewBot(tele.Settings{
		Token:   "token",
		URL:     server.URL,
		Client:  server.Client(),
		Offline: true,
	})
	require.NoError(t, err)

	sender := NewTelegramSender(botAPI)
	err = sender.Send(7, `Done. &lt;b&gt;file.txt&lt;/b&gt;`)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, calls, 1)
	require.Equal(t, "Done. <b>file.txt</b>", calls[0]["text"])
	require.Equal(t, string(tele.ModeHTML), calls[0]["parse_mode"])
}

func TestTelegramSender_SendFallsBackToSanitizedHTML(t *testing.T) {
	t.Parallel()

	var (
		mu    sync.Mutex
		calls []map[string]string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, r.Body.Close())

		var payload map[string]string
		err = json.Unmarshal(body, &payload)
		require.NoError(t, err)

		mu.Lock()
		calls = append(calls, payload)
		callIndex := len(calls)
		mu.Unlock()

		if callIndex == 1 {
			_, err = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: can't parse entities: unsupported start tag"}`))
			require.NoError(t, err)
			return
		}

		_, err = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":0,"chat":{"id":7,"type":"private"},"text":"ok"}}`))
		require.NoError(t, err)
	}))
	defer server.Close()

	botAPI, err := tele.NewBot(tele.Settings{
		Token:   "token",
		URL:     server.URL,
		Client:  server.Client(),
		Offline: true,
	})
	require.NoError(t, err)

	sender := NewTelegramSender(botAPI)
	err = sender.Send(7, `<div>Hello</div><b>world</b>`)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, calls, 2)
	require.Equal(t, `<div>Hello</div><b>world</b>`, calls[0]["text"])
	require.Equal(t, "Hello\n<b>world</b>", calls[1]["text"])
}

func TestTelegramSender_SendLeavesPlainTextUnchanged(t *testing.T) {
	t.Parallel()

	var text string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, r.Body.Close())

		var payload map[string]string
		err = json.Unmarshal(body, &payload)
		require.NoError(t, err)
		text = payload["text"]

		_, err = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":0,"chat":{"id":7,"type":"private"},"text":"ok"}}`))
		require.NoError(t, err)
	}))
	defer server.Close()

	botAPI, err := tele.NewBot(tele.Settings{
		Token:   "token",
		URL:     server.URL,
		Client:  server.Client(),
		Offline: true,
	})
	require.NoError(t, err)

	sender := NewTelegramSender(botAPI)
	err = sender.Send(7, "hello world")
	require.NoError(t, err)
	require.Equal(t, "hello world", text)
}
