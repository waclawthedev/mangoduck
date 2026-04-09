package bot

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type runnableBotStub struct {
	startCalled chan struct{}
	stopCalled  chan struct{}
	startDone   chan struct{}
	stopFunc    func()
}

func (s *runnableBotStub) Start() {
	close(s.startCalled)
	<-s.startDone
}

func (s *runnableBotStub) Stop() {
	close(s.stopCalled)
	if s.stopFunc != nil {
		s.stopFunc()
	}
}

func TestRun_StopsBotWhenContextCancelled(t *testing.T) {
	var stub runnableBotStub
	stub.startCalled = make(chan struct{})
	stub.stopCalled = make(chan struct{})
	stub.startDone = make(chan struct{})
	stub.stopFunc = func() {
		close(stub.startDone)
	}

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan struct{})

	go func() {
		defer close(runDone)
		Run(ctx, &stub)
	}()

	select {
	case <-stub.startCalled:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for bot start")
	}

	cancel()

	select {
	case <-stub.stopCalled:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for bot stop")
	}

	select {
	case <-runDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for runner exit")
	}
}

func TestRun_WaitsForBotStartLoopToExitAfterStop(t *testing.T) {
	var stub runnableBotStub
	stub.startCalled = make(chan struct{})
	stub.stopCalled = make(chan struct{})
	stub.startDone = make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan struct{})

	go func() {
		defer close(runDone)
		Run(ctx, &stub)
	}()

	<-stub.startCalled
	cancel()
	<-stub.stopCalled

	select {
	case <-runDone:
		require.Fail(t, "runner should wait for bot start loop to exit")
	default:
	}

	close(stub.startDone)

	select {
	case <-runDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for runner exit")
	}
}
