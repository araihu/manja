package application

import "fmt"

type ErrorKind string

const (
	ErrorDependency  ErrorKind = "dependency"
	ErrorValidation  ErrorKind = "validation"
	ErrorInput       ErrorKind = "input"
	ErrorSource      ErrorKind = "source"
	ErrorParse       ErrorKind = "parse"
	ErrorIntegrity   ErrorKind = "integrity"
	ErrorEvaluation  ErrorKind = "evaluation"
	ErrorTransaction ErrorKind = "transaction"
	ErrorCache       ErrorKind = "cache"
)

type Error struct {
	Kind      ErrorKind
	Operation string
	Err       error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Operation == "" {
		return e.Err.Error()
	}
	return e.Operation + ": " + e.Err.Error()
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func wrapError(kind ErrorKind, operation string, err error) error {
	if err == nil {
		return nil
	}
	return &Error{Kind: kind, Operation: operation, Err: err}
}

func dependencyError(operation, message string) error {
	return wrapError(ErrorDependency, operation, fmt.Errorf("%s", message))
}

func validationError(operation, message string) error {
	return wrapError(ErrorValidation, operation, fmt.Errorf("%s", message))
}
