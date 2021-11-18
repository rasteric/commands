package commands

import (
	"errors"
	"sync"
)

var ErrOutOfMemory = errors.New("command storage limit exceeded; try to increase the undo/redo limit")
var ErrToManyConfig = errors.New("only one optional configuration argument can be passed to NewCmdMgr")
var ErrAttemptReuseNumeric = errors.New("attempt to register a numeric command ID already in use")

// Command represents a type of command (without specific application data).
type Cmd struct {
	sort         int    // the numeric sort of the command (unique)
	name         string // name of the command suitable for menus, not unique
	info         string // short explanation of the command, not unique
	menuShortcut string // the menu shortcut, if there is any, "" otherwise (unique)
}

// NewCmd returns a new command.
func NewCmd(sort int, name, info, shortcut string) *Cmd {
	return &Cmd{sort: sort, info: info, name: name, menuShortcut: shortcut}
}

// Name returns the name of a command.
func (c *Cmd) Name() string { return c.name }

// ID returns the numeric sort of a command.
func (c *Cmd) ID() int { return c.sort }

// Info returns a short info string for the command.
func (c *Cmd) Info() string { return c.info }

// Shortcut returns the menu shortcut of the command.
func (c *Cmd) Shortcut() string { return c.menuShortcut }

// Op holds operations that can be executed asynchronously.
type Op struct {
	id        int           // ID of this instance (unique at runtime)
	sort      Command       // the type and general properties of the command
	args      []interface{} // the arguments that this operation takes
	proc      Fn            // the actual operation function
	final     Fn            // the final result in case the operation is async
	undo      Fn            // the function to undo the operation
	undoArgs  []interface{} // the arguments for the undo function
	undoFinal Fn            // the function called when undoing has finished
}

// NewOp creates a new Op with given ID and data.
func NewOp(id int, sort Command, args []interface{}, proc Fn, final Fn, undo Fn,
	undoArgs []interface{}, undoFinal Fn) *Op {
	return &Op{
		id:        id,
		sort:      sort,
		args:      args,
		proc:      proc,
		final:     final,
		undo:      undo,
		undoArgs:  undoArgs,
		undoFinal: undoFinal,
	}
}

// ID returns the ID of that operation, which is runtime-unique.
func (o *Op) ID() int { return o.id }

// Sort returns the command sort of the operation.
func (o *Op) Sort() Command { return o.sort }

// Args returns the arguments of the operation.
func (o *Op) Args() []interface{} { return o.args }

// Proc returns the procedure that executes the operation.
func (o *Op) Proc() Fn { return o.proc }

// Final returns the procedure that is called once command execution is finished.
func (o *Op) Final() Fn { return o.final }

// Undo returns the procedure that is called to undo the effects of the operation.
func (o *Op) Undo() Fn { return o.undo }

// UndoFinal returns the procedure that is called when the operation has been undone.
func (o *Op) UndoFinal() Fn { return o.undoFinal }

// OpManager manages commands and provides undo/redo functionality.
type OpManager struct {
	InProgress []int // holds operations in progress
	Done       []int // holds operations that have been done
	Redoable   []int // holds operations that have been undone and can be redone
	// unexported private fields
	mutex      sync.Mutex
	cmdIDCount int
	opIDCount  int
	opStorage  map[int]Operation
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
	if len(config) > 0 {
		cfg = config[0]
	} else {
		cfg = Defaults
	}
	return &OpManager{
		InProgress: make([]int, 0),
		Done:       make([]int, 0),
		Redoable:   make([]int, 0),
		opStorage:  make(map[int]Operation),
		config:     cfg,
	}, nil
}

// NewCmdID returns a new, unused numeric command sort.
func (mgr *OpManager) NewCmdID() int {
	mgr.mutex.Lock()
	defer mgr.mutex.Unlock()
	mgr.cmdIDCount++
	return mgr.cmdIDCount
}

// RegisterCmdIDs registers a number of numeric command types for use. An error is returned
// if any of them has been used already. NewNumeric() will return higher command Numerics
// than the highest registered one afterwards.
func (mgr *OpManager) RegisterCmdIDs(cmdIDs ...int) error {
	mgr.mutex.Lock()
	defer mgr.mutex.Unlock()
	var m int
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
func (mgr *OpManager) NewOpID() int {
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
