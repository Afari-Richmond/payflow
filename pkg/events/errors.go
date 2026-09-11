package events

import "fmt"

// PermanentError marks a message-processing failure that will never
// succeed on retry (malformed payload, unknown aggregate). Consumers
// use this to decide dead-lettering (permanent) vs. requeueing
// (transient, the default for a plain error) — see
// docs/adr/003-rabbitmq-for-domain-events.md.
type PermanentError struct {
	Err error
}

func (e *PermanentError) Error() string {
	return fmt.Sprintf("permanent: %s", e.Err)
}

func (e *PermanentError) Unwrap() error {
	return e.Err
}

// NewPermanentError wraps err as a PermanentError.
func NewPermanentError(err error) *PermanentError {
	return &PermanentError{Err: err}
}
