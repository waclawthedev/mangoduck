package bot

import (
	"reflect"
	"testing"

	"mangoduck/internal/config"

	tele "gopkg.in/telebot.v4"
)

func TestRegisterHandlersRegistersPhotoChatHandler(t *testing.T) {
	t.Parallel()

	botAPI, err := tele.NewBot(tele.Settings{Offline: true})
	if err != nil {
		t.Fatalf("creating offline bot: %v", err)
	}

	registerHandlers(botAPI, config.Config{}, nil, nil, nil, nil)

	handlersField := reflect.ValueOf(botAPI).Elem().FieldByName("handlers")
	if !handlersField.IsValid() {
		t.Fatal("handlers field is missing")
	}

	if !handlersField.MapIndex(reflect.ValueOf(tele.OnText)).IsValid() {
		t.Fatal("expected OnText handler to be registered")
	}

	if !handlersField.MapIndex(reflect.ValueOf(tele.OnPhoto)).IsValid() {
		t.Fatal("expected OnPhoto handler to be registered")
	}
}
