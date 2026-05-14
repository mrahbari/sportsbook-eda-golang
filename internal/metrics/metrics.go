package metrics

import "expvar"

// OutboxPublished counts successful outbox rows marked published (after broker publish + DB commit).
var OutboxPublished = expvar.NewInt("outbox_events_published_total")

// ConsumerHandlerErrors counts handler errors (before NACK) across consumers (optional increments from callers).
var ConsumerHandlerErrors = expvar.NewInt("consumer_handler_errors_total")
