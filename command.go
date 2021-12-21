package commands

import (
	"context"
	"errors"
	"sync"
)

var ErrOutOfMemory = errors.New("command storage limit exceeded; try to increase the undo/redo limit")
var ErrToManyConfig = errors.New("only one optional configuration argument can be passed to NewCmdMgr")

// OpManager manages commands and provides undo/redo functionality.
type OpManager struct {
	Undoable []Operation // holds operations that have been done
	Redoable []Operation // holds operations that have been undone and can be redone
	// unexported private fields
	mutex  sync.Mutex
	config Config
}

// UnlimitedStorage is an option for NewCmdMgr that allows for unlimited storage.
const UnlimitedStorage = 0

// Config represents a CmdMgr configuration.
type Config struct {
	StorageLimit int
}

// Defaults represents the default configuration of an OpManager. Use the Defaults as a starting
// point for modifications instead of an empty Config.
var Defaults = Config{}

// NewOpManager returns a new, empty operations manager.
func NewOpManager(config ...Config) (*OpManager, error) {
	if len(config) > 1 {
		return nil, ErrToManyConfig
	}
	var cfg Config
	if len(config) > 0 {
		cfg = config[0]
	} else {
		cfg = Defaults
	}
	return &OpManager{
		Undoable: make([]Operation, 0),
		Redoable: make([]Operation, 0),
		config:   cfg,
	}, nil
}

func (mgr *OpManager) hasBeenDone(op Operation) {
	mgr.mutex.Lock()
	defer mgr.mutex.Unlock()
	mgr.Undoable = append(mgr.Undoable, op)
}

func (mgr *OpManager) hasBeenUndone(op Operation) {
	mgr.mutex.Lock()
	defer mgr.mutex.Unlock()
	for i, o := range mgr.Undoable {
		if o == op {
			mgr.Undoable = append(mgr.Undoable[:i], mgr.Undoable[i+1:]...)
			break
		}
	}
	mgr.Redoable = append(mgr.Redoable, op)
}

func (mgr *OpManager) hasBeenRedone(op Operation) {
	mgr.mutex.Lock()
	defer mgr.mutex.Unlock()
	for i, o := range mgr.Redoable {
		if o == op {
			mgr.Redoable = append(mgr.Redoable[:i], mgr.Redoable[i+1:]...)
			break
		}
	}
	mgr.Undoable = append(mgr.Undoable, op)
}

// Execute executes operation with the given arguments, taking care of the undo and redo history.
func (mgr *OpManager) Execute(ctx context.Context, op Operation, final func(result interface{}, err error)) {
	go func(ctx context.Context, op Operation, final func(result interface{}, err error)) {
		result, err := op.Execute(ctx)
		if err == nil {
			mgr.hasBeenDone(op)
		}
		final(result, err)
	}(ctx, op, final)
}

// Undo undos the operation. Any undo data must be stored in the operation itself.
func (mgr *OpManager) Undo(ctx context.Context, op Operation, final func(result interface{}, err error)) {
	go func(ctx context.Context, op Operation, final func(result interface{}, err error)) {
		result, err := op.Undo(ctx)
		if err == nil {
			mgr.hasBeenUndone(op)
		}
		final(result, err)
	}(ctx, op, final)
}

// Redo redos the operation.
func (mgr *OpManager) Redo(ctx context.Context, op Operation, final func(result interface{}, err error)) {
	go func(ctx context.Context, op Operation, final func(result interface{}, err error)) {
		result, err := op.Redo(ctx)
		if err == nil {
			mgr.hasBeenRedone(op)
		}
		final(result, err)
	}(ctx, op, final)
}

// CanUndo returns true if an operation can be undone.
func (mgr *OpManager) CanUndo() bool {
	return len(mgr.Undoable) > 0
}

// CanRedo returns true if an operation can be redone.
func (mgr *OpManager) CanRedo() bool {
	return len(mgr.Redoable) > 0
}

// UndoCmd returns the last command that can be undone, or nil if there is none.
func (mgr *OpManager) UndoCmd() Command {
	if len(mgr.Undoable) == 0 {
		return nil
	}
	return mgr.Undoable[len(mgr.Undoable)-1].Cmd()
}

// RedoCmd returns the last command that can be redone, or nil if there is none.
func (mgr *OpManager) RedoCmd() Command {
	if len(mgr.Redoable) == 0 {
		return nil
	}
	return mgr.Redoable[len(mgr.Redoable)-1].Cmd()
}
