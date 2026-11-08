package eventstore

import "errors"

var ErrConcurrencyConflict = errors.New("concurrency conflict occurred")
