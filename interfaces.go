package commands

// Proc represents a generic function call for operations.
type Fn func(op Operation, args []interface{}) (interface{}, error)

// FinalProc represents a generic final callback when a command operation has finished.
type FinalFn func(op Operation) (interface{}, error)

// UndoProc represents a generic callback for undoing the effects of the operation.
type UndoFn Fn

// UndoFinal represents a generic undo final callback when undoing has finished.
type UndoFinalFn FinalFn

// Command is an interface that represents a generic command.
type Command interface {
	ID() int              // the numeric command sort
	Name() string         // the name of the command
	Info() string         // an info describing the command
	MenuShortcut() string // the associated menu shortcut
}

// Operation is an interface representing a synchronous or asynchronous command application.
// It contains the data needed for executing the command.
type Operation interface {
	ID() int                 // the ID of the operation
	Sort() Command           // the command sort of the operation
	Args() []interface{}     // the arguments of the operation
	Proc() Fn                // the actual function of the operation
	Final() FinalFn          // the function that is called once the operation is completed
	Undo() UndoFn            // the function to undo the operation
	UndoArgs() []interface{} // the arguments to the function to undo the operation
	UndoFinal() UndoFinalFn  // the final function to be called once the operation has been undone
}
