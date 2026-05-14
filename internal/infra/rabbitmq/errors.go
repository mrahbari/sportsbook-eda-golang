package rabbitmq

import "errors"

// ErrPoison marks handler errors that should not get pre-NACK backoff (invalid JSON, schema mismatch).
// Such messages go straight to DLQ (design §6.3 poison path).
var ErrPoison = errors.New("permanent poison message")

// ErrTransient hints that a short backoff before NACK may help (e.g. optimistic lock conflict).
var ErrTransient = errors.New("transient consumer failure")
