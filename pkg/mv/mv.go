package mv

import (
	"errors"
	"sync"

	"github.com/immutable/metavisor-cli/pkg/logging"
)

var (
	// ErrInterrupted is returned if the context is cancelled during execution
	ErrInterrupted = errors.New("the command was interrupted")
)

// MaybeString encapsulates a string and an eventual error. For use with
// result channels
type MaybeString struct {
	Result string
	Error  error
}

type cleanupFunc struct {
	f          func()
	onlyOnFail bool
}

var stackedCleanups *cleanupStack

// QueueCleanup will queue a cleanup function to be ran. If onlyOnFail is set,
// the function will only be ran if Cleanup is ran with failed = true. The
// functions are put in a stack, so the latest function put in will be ran first
func QueueCleanup(f func(), onlyOnFail bool) {
	if stackedCleanups == nil {
		stackedCleanups = &cleanupStack{
			mutex: sync.Mutex{},
			funcs: make([]cleanupFunc, 0),
		}
	}
	stackedCleanups.Push(cleanupFunc{
		f:          f,
		onlyOnFail: onlyOnFail,
	})
}

// Cleanup will run all queued cleanup functions
func Cleanup(success bool) {
	if stackedCleanups == nil {
		return
	}
	cleaned := false
	for stackedCleanups.Len() != 0 {
		f, err := stackedCleanups.Pop()
		if err != nil {
			// Stack is empty, shouldn't happen with the check in the loop
			break
		}
		if !success || (success && !f.onlyOnFail) {
			logging.Debug("Running cleanup function")
			cleaned = true
			f.f()
		}
	}
	if cleaned {
		logging.Info("Cleanup completed")
	}
}

// A simple thread safe stack to store the cleanup functions
type cleanupStack struct {
	mutex sync.Mutex
	funcs []cleanupFunc
	size  int
}

func (s *cleanupStack) Push(f cleanupFunc) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.funcs = append(s.funcs, f)
	s.size++
}

func (s *cleanupStack) Pop() (cleanupFunc, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.size == 0 {
		return cleanupFunc{}, errors.New("the stack is empty")
	}
	f := s.funcs[s.size-1]
	s.funcs = s.funcs[:s.size-1]
	s.size--
	return f, nil
}

func (s *cleanupStack) Len() int {
	return s.size
}
