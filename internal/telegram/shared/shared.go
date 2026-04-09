package shared

import (
	"errors"

	tele "gopkg.in/telebot.v4"
)

//go:generate mockery

// Context is a wrapper interface for tele.Context to enable mock generation via GoLand / mockery.
type Context = tele.Context

var ErrResponseHandled = errors.New("response handled")
