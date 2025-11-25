package runner

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/rlch/scaf"
)

// Runner executes scaf test suites.
type Runner struct {
	dialect  scaf.Dialect
	handler  Handler
	failFast bool
	filter   *regexp.Regexp
}

// Option configures a Runner.
type Option func(*Runner)

// WithDialect sets the database dialect.
func WithDialect(d scaf.Dialect) Option {
	return func(r *Runner) {
		r.dialect = d
	}
}

// WithHandler sets the event handler.
func WithHandler(h Handler) Option {
	return func(r *Runner) {
		r.handler = h
	}
}

// WithFailFast stops on first failure.
func WithFailFast(enabled bool) Option {
	return func(r *Runner) {
		r.failFast = enabled
	}
}

// WithFilter sets a regex pattern to filter which tests run.
// Tests whose path matches the pattern will be executed.
func WithFilter(pattern string) Option {
	return func(r *Runner) {
		if pattern != "" {
			r.filter = regexp.MustCompile(pattern)
		}
	}
}

// New creates a Runner with the given options.
func New(opts ...Option) *Runner {
	r := &Runner{}
	for _, opt := range opts {
		opt(r)
	}

	return r
}

// Run executes a parsed Suite and returns the results.
func (r *Runner) Run(ctx context.Context, suite *scaf.Suite, suitePath string) (*Result, error) {
	if r.dialect == nil {
		return nil, ErrNoDialect
	}

	result := NewResult()

	handlers := []Handler{NewResultHandler()}
	if r.handler != nil {
		handlers = append(handlers, r.handler)
	}

	if r.failFast {
		handlers = append(handlers, NewStopOnFailHandler(1))
	}

	handler := NewMultiHandler(handlers...)

	if suite.Setup != nil {
		if err := r.executeSetup(ctx, *suite.Setup); err != nil {
			return result, err
		}
	}

	for _, scope := range suite.Scopes {
		err := r.runQueryScope(ctx, scope, suitePath, handler, result)
		if errors.Is(err, ErrMaxFailures) {
			break
		}

		if err != nil {
			return result, err
		}
	}

	result.Finish()

	return result, nil
}

func (r *Runner) runQueryScope(
	ctx context.Context,
	scope *scaf.QueryScope,
	suitePath string,
	handler Handler,
	result *Result,
) error {
	if scope.Setup != nil {
		if err := r.executeSetup(ctx, *scope.Setup); err != nil {
			return err
		}
	}

	for _, item := range scope.Items {
		path := []string{scope.QueryName}

		var err error

		switch {
		case item.Test != nil:
			err = r.runTest(ctx, item.Test, path, suitePath, handler, result)
		case item.Group != nil:
			err = r.runGroup(ctx, item.Group, path, suitePath, handler, result)
		}

		if errors.Is(err, ErrMaxFailures) {
			return err
		}
	}

	return nil
}

func (r *Runner) runGroup(
	ctx context.Context,
	group *scaf.Group,
	parentPath []string,
	suitePath string,
	handler Handler,
	result *Result,
) error {
	path := make([]string, len(parentPath)+1)
	copy(path, parentPath)
	path[len(parentPath)] = group.Name

	if group.Setup != nil {
		if err := r.executeSetup(ctx, *group.Setup); err != nil {
			return err
		}
	}

	for _, item := range group.Items {
		var err error

		switch {
		case item.Test != nil:
			err = r.runTest(ctx, item.Test, path, suitePath, handler, result)
		case item.Group != nil:
			err = r.runGroup(ctx, item.Group, path, suitePath, handler, result)
		}

		if errors.Is(err, ErrMaxFailures) {
			return err
		}
	}

	return nil
}

func (r *Runner) runTest(
	ctx context.Context,
	test *scaf.Test,
	parentPath []string,
	suitePath string,
	handler Handler,
	result *Result,
) error {
	path := make([]string, len(parentPath)+1)
	copy(path, parentPath)
	path[len(parentPath)] = test.Name

	// Check if test matches filter
	if !r.matchesFilter(path) {
		return nil
	}

	start := time.Now()

	_ = handler.Event(ctx, Event{
		Time:   start,
		Action: ActionRun,
		Suite:  suitePath,
		Path:   path,
	}, result)

	if test.Setup != nil {
		if err := r.executeSetup(ctx, *test.Setup); err != nil {
			return r.emitError(ctx, path, suitePath, start, err, handler, result)
		}
	}

	// Build params and expectations from statements
	params := make(map[string]any)
	expectations := make(map[string]any)

	for _, stmt := range test.Statements {
		if len(stmt.Key) > 0 && stmt.Key[0] == '$' {
			params[stmt.Key[1:]] = stmt.Value.ToGo()
		} else {
			expectations[stmt.Key] = stmt.Value.ToGo()
		}
	}

	// TODO(query): Execute query and compare results
	_ = params
	_ = expectations

	elapsed := time.Since(start)

	return handler.Event(ctx, Event{
		Time:    time.Now(),
		Action:  ActionPass,
		Suite:   suitePath,
		Path:    path,
		Elapsed: elapsed,
	}, result)
}

func (r *Runner) executeSetup(ctx context.Context, query string) error {
	_, err := r.dialect.Execute(ctx, query, nil)

	return err
}

func (r *Runner) emitError(
	ctx context.Context,
	path []string,
	suitePath string,
	start time.Time,
	err error,
	handler Handler,
	result *Result,
) error {
	return handler.Event(ctx, Event{
		Time:    time.Now(),
		Action:  ActionError,
		Suite:   suitePath,
		Path:    path,
		Elapsed: time.Since(start),
		Error:   err,
	}, result)
}

// matchesFilter returns true if the test path matches the filter pattern.
// If no filter is set, all tests match.
func (r *Runner) matchesFilter(path []string) bool {
	if r.filter == nil {
		return true
	}

	pathStr := strings.Join(path, "/")

	return r.filter.MatchString(pathStr)
}
