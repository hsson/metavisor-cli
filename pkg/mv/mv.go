package mv

import (
	"errors"
	"sync"

	"github.com/brkt/metavisor-cli/pkg/logging"
)

var (
	// ErrInterupted is returned if the context is cancelled during execution
	ErrInterupted = errors.New("the command was interupted")
)

// MaybeString encapsulates a string and an eventual error
type MaybeString struct {
	Result string
	Error  error
}

var queuedCleanups []cleanupFunc

type cleanupFunc struct {
	f          func()
	onlyOnFail bool
}

// Cleanup will run all queued cleanup functions
func Cleanup(success bool) {
	cleaned := false
	var wg sync.WaitGroup
	for i := range queuedCleanups {
		wg.Add(1)
		go func(index int) {
			if !success {
				logging.Debug("Running cleanup function")
				cleaned = true
				queuedCleanups[index].f()
			} else if success && !queuedCleanups[index].onlyOnFail {
				logging.Debug("Running cleanup function")
				cleaned = true
				queuedCleanups[index].f()
			}
			wg.Done()
		}(i)
	}
	wg.Wait()
	if cleaned {
		logging.Info("Cleanup completed")
	}
}

// QueueCleanup will queue a cleanup function to be ran. If onlyOnFail is set,
// the function will only be ran if Cleanup is ran with failed = true
func QueueCleanup(f func(), onlyOnFail bool) {
	if queuedCleanups == nil {
		queuedCleanups = []cleanupFunc{}
	}
	queuedCleanups = append(queuedCleanups, cleanupFunc{
		f:          f,
		onlyOnFail: onlyOnFail,
	})
}
