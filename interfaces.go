package commands

import "context"

// Command is an interface that represents a generic command.
type Command interface {
	Name() string     // the name of the command
	Info() string     // an info describing the command
	Shortcut() string // the associated menu shortcut as string
}

// Operation is an interface representing asynchronous command application.
type Operation interface {
	Cmd() Command
	Execute(ctx context.Context) (any, error)
	Undo(ctx context.Context) (any, error)
	Redo(ctx context.Context) (any, error)
}
