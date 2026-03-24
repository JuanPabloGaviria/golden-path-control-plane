package app

type ValidationError struct {
	Err error
}

func (e ValidationError) Error() string {
	return e.Err.Error()
}

func (e ValidationError) Unwrap() error {
	return e.Err
}

type StateConflictError struct {
	Err error
}

func (e StateConflictError) Error() string {
	return e.Err.Error()
}

func (e StateConflictError) Unwrap() error {
	return e.Err
}
