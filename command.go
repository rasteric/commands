package commands

import (
	"context"
	"errors"
	"sync"
)

var ErrOutOfMemory = errors.New("command storage limit exceeded; try to increase the undo/redo limit")
var ErrTooManyConfig = errors.New("only one optional configuration argument can be passed to NewCmdMgr")

// UnlimitedStorage is an option for NewCmdMgr that allows for unlimited storage.
const UnlimitedStorage = 0

// Config represents a CmdMgr configuration.
type Config struct {
	StorageLimit int
}

// Cancelation represents a cancel function for an operation. This is only used internally.
type Cancelation struct {
	id  int
	ctx context.Context
	f   func()
	mgr *OpManager
}

// Cancel cancels the operation of this cancelation.
func (c Cancelation) Cancel() {
	c.f()
	if c.mgr != nil {
		c.mgr.removeCancelation(c)
	}
}

// Context returns the context created from this cancelation.
func (c Cancelation) Context() context.Context {
	return c.ctx
}

// Defaults represents the default configuration of an OpManager. Use the Defaults as a starting
// point for modifications instead of an empty Config.
var Defaults = Config{}

// OpManager manages commands and provides undo/redo functionality.
type OpManager struct {
	undoable     []Operation // holds operations that have been done
	redoable     []Operation // holds operations that have been undone and can be redone
	config       Config
	mutex        sync.RWMutex   // internal sync
	wg           sync.WaitGroup // for waiting until everything has finished
	cancelations []Cancelation  // for canceling pending operations
}

// NewOpManager returns a new, empty operations manager.
func NewOpManager(config ...Config) (*OpManager, error) {
	if len(config) > 1 {
		return nil, ErrTooManyConfig
	}
	var cfg Config
	if len(config) > 0 {
		cfg = config[0]
	} else {
		cfg = Defaults
	}
	return &OpManager{
		undoable: make([]Operation, 0),
		redoable: make([]Operation, 0),
		config:   cfg,
	}, nil
}

func (mgr *OpManager) hasBeenDone(op Operation) {
	mgr.mutex.Lock()
	defer mgr.mutex.Unlock()
	mgr.undoable = append(mgr.undoable, op)
}

func (mgr *OpManager) hasBeenUndone(op Operation) {
	mgr.mutex.Lock()
	defer mgr.mutex.Unlock()
	for i, o := range mgr.undoable {
		if o == op {
			mgr.undoable = append(mgr.undoable[:i], mgr.undoable[i+1:]...)
			break
		}
	}
	mgr.redoable = append(mgr.redoable, op)
}

// Reoables returns the redoable operations as slice.
func (mgr *OpManager) Redoables() []Operation {
	return mgr.redoable
}

// Undoables returns the undoable operations as slice.
func (mgr *OpManager) Undoables() []Operation {
	return mgr.undoable
}

func (mgr *OpManager) hasBeenRedone(op Operation) {
	mgr.mutex.Lock()
	defer mgr.mutex.Unlock()
	for i, o := range mgr.redoable {
		if o == op {
			mgr.redoable = append(mgr.redoable[:i], mgr.redoable[i+1:]...)
			break
		}
	}
	mgr.undoable = append(mgr.undoable, op)
}

// Execute executes an operation asynchronously, taking care of the undo and redo history.
func (mgr *OpManager) Execute(ctx context.Context, op Operation,
	final func(result interface{}, err error)) Cancelation {
	var cancel Cancelation
	go func(ctx context.Context, op Operation, final func(result interface{}, err error)) {
		mgr.wg.Add(1)
		defer mgr.wg.Done()
		cancel = mgr.withCancel(ctx)
		defer mgr.removeCancelation(cancel)
		result, err := op.Execute(ctx)
		if err == nil {
			mgr.hasBeenDone(op)
		}
		final(result, err)
	}(ctx, op, final)
	return cancel
}

// ExecuteSync executes an operation synchronously, returning the result or an error.
func (mgr *OpManager) ExecuteSync(ctx context.Context, op Operation) (interface{}, error) {
	result, err := op.Execute(ctx)
	if err == nil {
		mgr.hasBeenDone(op)
	}
	return result, err
}

// Undo undos the operation. Any undo data must be stored in the operation itself.
func (mgr *OpManager) Undo(ctx context.Context, op Operation,
	final func(result interface{}, err error)) Cancelation {
	var cancel Cancelation
	go func(ctx context.Context, op Operation, final func(result interface{}, err error)) {
		mgr.wg.Add(1)
		defer mgr.wg.Done()
		cancel = mgr.withCancel(ctx)
		defer mgr.removeCancelation(cancel)
		result, err := op.Undo(ctx)
		if err == nil {
			mgr.hasBeenUndone(op)
		}
		final(result, err)
	}(ctx, op, final)
	return cancel
}

// Redo redos the operation.
func (mgr *OpManager) Redo(ctx context.Context, op Operation,
	final func(result interface{}, err error)) Cancelation {
	var cancel Cancelation
	go func(ctx context.Context, op Operation, final func(result interface{}, err error)) {
		mgr.wg.Add(1)
		defer mgr.wg.Done()
		cancel = mgr.withCancel(ctx)
		defer mgr.removeCancelation(cancel)
		result, err := op.Redo(ctx)
		if err == nil {
			mgr.hasBeenRedone(op)
		}
		final(result, err)
	}(ctx, op, final)
	return cancel
}

// CanUndo returns true if an operation can be undone.
func (mgr *OpManager) CanUndo() bool {
	return len(mgr.undoable) > 0
}

// CanRedo returns true if an operation can be redone.
func (mgr *OpManager) CanRedo() bool {
	return len(mgr.redoable) > 0
}

// UndoCmd returns the last command that can be undone, or nil if there is none.
func (mgr *OpManager) UndoCmd() Command {
	if len(mgr.undoable) == 0 {
		return nil
	}
	return mgr.undoable[len(mgr.undoable)-1].Cmd()
}

// RedoCmd returns the last command that can be redone, or nil if there is none.
func (mgr *OpManager) RedoCmd() Command {
	if len(mgr.redoable) == 0 {
		return nil
	}
	return mgr.redoable[len(mgr.redoable)-1].Cmd()
}

// WithCancel returns a new cancelation for an operation. This can later be used to
// cancel the operation.
func (mgr *OpManager) withCancel(ctx context.Context) Cancelation {
	mgr.mutex.Lock()
	mgr.mutex.Unlock()
	n := len(mgr.cancelations) + 1
	c, done := context.WithCancel(ctx)
	cancelation := Cancelation{id: n, ctx: c, f: done}
	mgr.addCancelation(cancelation)
	return cancelation
}

// addCancelation adds a cancelation function for an operation so it can be called later to
// cancel the operation.
func (mgr *OpManager) addCancelation(c Cancelation) {
	mgr.cancelations = append(mgr.cancelations, c)
}

// removeCancelation removes a cancelation function for an operation when it is no longer needed.
// It is no longer needed when it has been called.
func (mgr *OpManager) removeCancelation(c Cancelation) {
	mgr.mutex.Lock()
	defer mgr.mutex.Unlock()
	for i, g := range mgr.cancelations {
		if g.id == c.id {
			mgr.cancelations[i] = mgr.cancelations[len(mgr.cancelations)-1]
			mgr.cancelations = mgr.cancelations[:len(mgr.cancelations)-1]
		}
	}
}

// CancelAll cancels all pending operations.
func (mgr *OpManager) CancelAll() {
	mgr.mutex.RLock()
	defer mgr.mutex.RUnlock()
	for _, c := range mgr.cancelations {
		c.Cancel()
	}
}

// WaitAll waits for all pending operations to finish.
func (mgr *OpManager) WaitAll() {
	mgr.wg.Wait()
}

// Shutdown shuts down the op manager, waiting for all pending operations to finish.
// If cancel is true, then running operations are canceled, otherwise the op manager
// allows them to finish first. Operations should always make sure that they cancel
// gracefully and as fast as possible.
func (mgr *OpManager) Shutdown(cancel bool) {
	if cancel {
		mgr.CancelAll()
	}
	mgr.WaitAll()
}
