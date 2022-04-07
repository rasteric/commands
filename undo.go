package undo

import (
	"context"
	"errors"
	"sync"
)

var ErrOutOfMemory = errors.New("command storage limit exceeded; try to increase the undo/redo limit")
var ErrTooManyConfig = errors.New("only one optional configuration argument can be passed to UndoManager")
var ErrCantUndo = errors.New("cannot undo operation - nothing to undo")
var ErrCantRedo = errors.New("cannot redo operation - nothing to redo")

// UnlimitedStorage is an option for NewCmdMgr that allows for unlimited storage.
const UnlimitedStorage = 0

// Config represents a CmdMgr configuration.
type Config struct {
	StorageLimit int
}

// Defaults represents the default configuration of an OpManager. Use the Defaults as a starting
// point for modifications instead of an empty Config.
var Defaults = Config{}

// op is used to internally store functions with names. In case of an undo operation, op stores the
// undo function fn and the redo function redoFn. Used on the redo stack, however, the op only uses
// fn to store the redo function and redoFn will be nil.
type op struct {
	fn     func(ctx context.Context) error // the undo function
	redoFn func(ctx context.Context) error // a function to redo the function that was undone
	name   string                          // the name used in undo and redo templates
}

// UndoManager manages commands and provides undo/redo functionality.
type UndoManager struct {
	undoStack  []op            // holds undo operations (op.redoFn holds the redo function)
	redoStack  []op            // holds redo operations (op.redoFn is nil)
	config     Config          // the undo manager configuration
	mutex      sync.RWMutex    // internal sync
	wg         sync.WaitGroup  // for waiting until everything has finished
	mainCtx    context.Context // the master context from which other contexts need to be derived
	mainCancel func()          // the main cancel function that cancels all pending operations
}

// New returns a new, empty undo manager. undoMsg and redoMsg are fmt templates which
// take the name of an operation as argument and yield a short message to use e.g. in menus.
func New(config ...Config) (*UndoManager, error) {
	if len(config) > 1 {
		return nil, ErrTooManyConfig
	}
	var cfg Config
	if len(config) > 0 {
		cfg = config[0]
	} else {
		cfg = Defaults
	}
	mgr := &UndoManager{
		undoStack: make([]op, 0),
		redoStack: make([]op, 0),
		config:    cfg,
	}
	mgr.mainCtx, mgr.mainCancel = context.WithCancel(context.Background())
	return mgr, nil
}

// WithCancel returns a cancelable context based on the UndoManager's master context.
func (mgr *UndoManager) WithCancel() (context.Context, func()) {
	return context.WithCancel(mgr.mainCtx)
}

// Context returns the cancelable master context.
func (mgr *UndoManager) Context() context.Context {
	return mgr.mainCtx
}

// WGAdd adds n entries to the UndoManager's wait group.
func (mgr *UndoManager) WGAdd(n int) {
	mgr.wg.Add(n)
}

// CancelAll cancels all pending operations.
func (mgr *UndoManager) CancelAll() {
	mgr.mutex.RLock()
	defer mgr.mutex.RUnlock()
	mgr.mainCancel()
}

// WaitAll waits for all pending operations to finish.
func (mgr *UndoManager) WaitAll() {
	mgr.wg.Wait()
}

// Shutdown shuts down the op manager, waiting for all pending operations to finish.
// If cancel is true, then running operations are canceled, otherwise the op manager
// allows them to finish first. Operations should always make sure that they cancel
// gracefully and as fast as possible.
func (mgr *UndoManager) Shutdown(cancel bool) {
	if cancel {
		mgr.CancelAll()
	}
	mgr.WaitAll()
}

// Add adds an undo function to the UndoManager.
func (mgr *UndoManager) Add(name string, undoFn func(ctx context.Context) error,
	redoFn func(ctx context.Context) error) {
	mgr.mutex.Lock()
	defer mgr.mutex.Unlock()
	mgr.undoStack = append(mgr.undoStack, op{name: name, fn: undoFn, redoFn: redoFn})
}

// CanUndo returns true if an operation can be undone, false otherwise.
func (mgr *UndoManager) CanUndo() bool {
	mgr.mutex.RLock()
	defer mgr.mutex.RUnlock()
	return len(mgr.undoStack) > 0
}

// UndoName returns the name of the function to undo, "" if there is none.
func (mgr *UndoManager) UndoName() string {
	mgr.mutex.RLock()
	defer mgr.mutex.RUnlock()
	if len(mgr.undoStack) == 0 {
		return ""
	}
	return mgr.undoStack[len(mgr.undoStack)-1].name
}

func (mgr *UndoManager) popUndo() (op, bool) {
	mgr.mutex.Lock()
	defer mgr.mutex.Unlock()
	if len(mgr.undoStack) == 0 {
		return op{}, false
	}
	undoOp := mgr.undoStack[len(mgr.undoStack)-1]
	mgr.undoStack = mgr.undoStack[:len(mgr.undoStack)-1]
	return undoOp, true
}

// Undo the last operation added to the UndoManager. If no operation can be undone, ErrCantUndo is returned.
func (mgr *UndoManager) Undo(ctx context.Context) error {
	o, ok := mgr.popUndo()
	if !ok {
		return ErrCantUndo
	}
	err := o.fn(ctx)
	if err != nil {
		return err
	}
	mgr.mutex.Lock()
	defer mgr.mutex.Unlock()
	mgr.redoStack = append(mgr.redoStack, op{name: o.name, fn: o.redoFn})
	return nil
}

// CanRedo returns true if an operation can be redone, false otherwise.
func (mgr *UndoManager) CanRedo() bool {
	mgr.mutex.RLock()
	defer mgr.mutex.RUnlock()
	return len(mgr.redoStack) > 0
}

// RedoName returns the name of the function to redo, "" if there is none.
func (mgr *UndoManager) RedoName() string {
	mgr.mutex.RLock()
	defer mgr.mutex.RUnlock()
	if len(mgr.redoStack) == 0 {
		return ""
	}
	return mgr.redoStack[len(mgr.redoStack)-1].name
}

func (mgr *UndoManager) popRedo() (op, bool) {
	mgr.mutex.Lock()
	defer mgr.mutex.Unlock()
	if len(mgr.redoStack) == 0 {
		return op{}, false
	}
	redoOp := mgr.redoStack[len(mgr.redoStack)-1]
	mgr.redoStack = mgr.redoStack[:len(mgr.redoStack)-1]
	return redoOp, true
}

// Redo the last operation added to the UndoManager. If no operation can be redone, ErrCantRedo is returned.
func (mgr *UndoManager) Redo(ctx context.Context) error {
	op, ok := mgr.popRedo()
	if !ok {
		return ErrCantRedo
	}
	return op.fn(ctx)
}
