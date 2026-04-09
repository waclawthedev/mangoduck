package bot

import "context"

type runnableBot interface {
	Start()
	Stop()
}

func Run(ctx context.Context, b runnableBot) {
	done := make(chan struct{})

	go func() {
		defer close(done)
		b.Start()
	}()

	<-ctx.Done()
	b.Stop()
	<-done
}
