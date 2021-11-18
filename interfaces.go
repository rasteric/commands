package commands

import "context"

// Fn represents a generic function call for operations.
type Fn func(ctx context.Context, op Operation) (interface{}, error)

// Command is an interface that represents a generic command.
type Command interface {
	ID() int          // the numeric command sort
	Name() string     // the name of the command
	Info() string     // an info describing the command
	Shortcut() string // the associated menu shortcut
}

// Operation is an interface representing a synchronous or asynchronous command application.
// It contains the data needed for executing the command.
type Operation interface {
	ID() int                 // the ID of the operation
	Sort() Command           // the command sort of the operation
	Args() []interface{}     // the arguments of the operation
	Proc() Fn                // the actual function of the operation
	Final() Fn               // the function that is called once the operation is completed
	Undo() Fn                // the function to undo the operation
	UndoArgs() []interface{} // the arguments to the function to undo the operation
	UndoFinal() Fn           // the final function to be called once the operation has been undone
}
