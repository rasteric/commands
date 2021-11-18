package projects

import (
	"errors"
	"sync"
)

var ErrOutOfMemory = errors.New("command storage limit exceeded; try to increase the undo/redo limit")
var ErrToManyConfig = errors.New("only one optional configuration argument can be passed to NewCmdMgr")
var ErrAttemptReuseNumeric = errors.New("attempt to register a command numeric already in use")

// Proc represents a generic function call for operations.
type Fn func(op Op, args []interface{}) (interface{}, error)

// FinalProc represents a generic final callback when a command operation has finished.
type FinalFn func(op Op) (interface{}, error)

// UndoProc represents a generic callback for undoing the effects of the operation.
type UndoFn Fn

// UndoFinal represents a generic undo final callback when undoing has finished.
type UndoFinalFn FinalFn

// CmdID is the numeric type of a command.
type CmdID int

// Cmd represents a type of command (without specific application data).
type Cmd struct {
	Sort         CmdID  // the numeric sort of the command (unique)
	Name         string // name of the command suitable for menus, not unique
	Info         string // short explanation of the command, not unique
	MenuShortcut string // the menu shortcut, if there is any, "" otherwise (unique)
}

// Command is an interface representing a generic command.
type Command interface {
	ID() CmdID
	Name() string
	Info() string
	MenuShortcut() string
}

// OpID is a runtime-unique, monotonically increasing ID of a particular Cmd instance.
type OpID int

// Op holds operations that can be executed asynchronously. They can only be executed
// by an OpManager.
type Op struct {
	ID        OpID          // ID of this instance (unique at runtime)
	Sort      Command       // the type and general properties of the command
	Args      []interface{} // the arguments that this operation takes
	Proc      Fn            // the actual operation function
	Final     FinalFn       // the final result in case the operation is async
	Undo      UndoFn        // the function to undo the operation
	UndoFinal UndoFinalFn   // the function called when undoing has finished
}

// Operation is an interface representing a synchroous or asynchronous command application.
// It contains the data needed for executing the command.
type Operation interface {
	ID() OpID
	Sort() Command
	Args() []interface{}
	Proc() Fn
	Final() FinalFn
	Undo() UndoFn
	UndoFinal() UndoFinalFn
}

// OpManager manages commands and provides undo/redo functionality.
type OpManager struct {
	InProgress []OpID // holds commands in progress
	Done       []OpID // holds commands that have been done
	Redoable   []OpID // holds commands that have been undone and can be redone
	// unexported private fields
	mutex      sync.Mutex
	cmdIDCount CmdID
	opIDCount  OpID
	opStorage  map[OpID]Operation
	config     Config
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
	if len(config) == 0 {
		cfg = config[0]
	} else {
		cfg = Defaults
	}
	return &OpManager{
		InProgress: make([]OpID, 0),
		Done:       make([]OpID, 0),
		Redoable:   make([]OpID, 0),
		opStorage:  make(map[OpID]Operation),
		config:     cfg,
	}, nil
}

// NewCmdID returns a new, unused numeric command sort.
func (mgr *OpManager) NewCmdID() CmdID {
	mgr.mutex.Lock()
	defer mgr.mutex.Unlock()
	mgr.cmdIDCount++
	return mgr.cmdIDCount
}

// RegisterCmdIDs registers a number of numeric command types for use. An error is returned
// if any of them has been used already. NewNumeric() will return higher command Numerics
// than the highest registered one afterwards.
func (mgr *OpManager) RegisterNumerics(cmdIDs ...CmdID) error {
	mgr.mutex.Lock()
	defer mgr.mutex.Unlock()
	var m CmdID
	for _, n := range cmdIDs {
		if n <= mgr.cmdIDCount {
			return ErrAttemptReuseNumeric
		}
		if n > m {
			m = n
		}
	}
	mgr.cmdIDCount = m + 1
	return nil
}

// NewOpID returns a new, unused Op ID.
func (mgr *OpManager) NewOpID() OpID {
	mgr.mutex.Lock()
	defer mgr.mutex.Unlock()
	mgr.opIDCount++
	return mgr.opIDCount
}

// Execute executes a command and returns any immediate error. If the command
// executes asynchonously, then the result of the command is only obtained once FinalProc
// is called, otherwise the result is returned immediately.  Once a command has finished,
// it is put on the Done chain. While it is in progress, it is on the InProgress chain.
func (mgr *OpManager) Execute(op Operation) error {
	mgr.mutex.Lock()
	defer mgr.mutex.Unlock()
	if mgr.config.StorageLimit > 0 && len(mgr.opStorage) > mgr.config.StorageLimit {
		return ErrOutOfMemory
	}
	mgr.opStorage[op.ID()] = op
	mgr.InProgress = append(mgr.InProgress, op.ID())
	return nil
}
