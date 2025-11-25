package runner

import "context"

// Handler receives test events during execution.
type Handler interface {
	// Event is called for each test event as it occurs.
	Event(ctx context.Context, event Event, result *Result) error

	// Err is called for non-test errors (stderr, infrastructure issues).
	Err(text string) error
}

// MultiHandler fans out events to multiple handlers.
type MultiHandler struct {
	handlers []Handler
}

// NewMultiHandler creates a handler that dispatches to multiple handlers.
func NewMultiHandler(handlers ...Handler) *MultiHandler {
	return &MultiHandler{handlers: handlers}
}

// Event dispatches to all handlers, stopping on first error.
func (m *MultiHandler) Event(ctx context.Context, event Event, result *Result) error {
	for _, h := range m.handlers {
		err := h.Event(ctx, event, result)
		if err != nil {
			return err
		}
	}

	return nil
}

// Err dispatches to all handlers.
func (m *MultiHandler) Err(text string) error {
	for _, h := range m.handlers {
		err := h.Err(text)
		if err != nil {
			return err
		}
	}

	return nil
}

// ResultHandler updates the Result accumulator from events.
type ResultHandler struct{}

// NewResultHandler creates a handler that accumulates results.
func NewResultHandler() *ResultHandler {
	return &ResultHandler{}
}

// Event updates the result accumulator.
func (h *ResultHandler) Event(_ context.Context, event Event, result *Result) error {
	if event.Action == ActionOutput {
		result.AddOutput(event)
	} else {
		result.Add(event)
	}

	return nil
}

// Err is a no-op for ResultHandler.
func (h *ResultHandler) Err(_ string) error {
	return nil
}

// StopOnFailHandler stops execution when max failures is reached.
type StopOnFailHandler struct {
	maxFails int
}

// NewStopOnFailHandler creates a handler that stops after n failures.
func NewStopOnFailHandler(maxFails int) *StopOnFailHandler {
	return &StopOnFailHandler{maxFails: maxFails}
}

// Event checks if we've hit max failures.
func (h *StopOnFailHandler) Event(_ context.Context, event Event, result *Result) error {
	if h.maxFails <= 0 {
		return nil
	}

	if event.Action == ActionFail || event.Action == ActionError {
		if result.Failed+result.Errors >= h.maxFails {
			return ErrMaxFailures
		}
	}

	return nil
}

// Err is a no-op.
func (h *StopOnFailHandler) Err(_ string) error {
	return nil
}
