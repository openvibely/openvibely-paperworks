package testsupport

import "fmt"

// MutationWarningLocation supplies explicit attribution; it does not infer a
// Reference stack frame from a Go callback. Supported ZIP warnings are UserWarning.
type MutationWarningLocation struct {
	Module string
	Line   int
}

type mutationWarningKey struct {
	MutationWarningLocation
	Message  string
	AllLines bool
}

// MutationWarningRegistry retains suppression state across mutation calls.
// It models the pinned CReference warn_explicit path with a supplied module
// registry. In that path both once and module suppress per module, not globally.
// It is caller-owned, not concurrency safe. Arbitrary Reference regex/category
// filters, interpreter globals and automatic stack attribution are not modeled.
type MutationWarningRegistry struct {
	action  string
	filters []compiledMutationFilter
	seen    map[mutationWarningKey]bool
}

// SetAction replaces the native filter list with one fallback action and resets
// suppression, even for the same action. Use AddFilter for filterwarnings-style
// insertion. Invalid actions leave existing state untouched.
func (r *MutationWarningRegistry) SetAction(action string) error {
	if !validMutationWarningAction(action) {
		return fmt.Errorf("unsupported warning action %q", action)
	}
	r.action = action
	r.filters = nil
	r.seen = make(map[mutationWarningKey]bool)
	return nil
}

// MutationWarningError preserves warning promotion as an explicit Go error.
type MutationWarningError struct{ Message string }

func (e *MutationWarningError) Error() string { return e.Message }

// Handler binds one warning site to the existing mutation policy callback.
// Suppression is committed before delivery, including when delivery fails,
// matching the pinned warnings implementation. Error promotion is never cached.
func (r *MutationWarningRegistry) Handler(location MutationWarningLocation, emit func(string) error) func(string) error {
	return func(message string) error {
		action := r.selectAction(location, message)
		key := mutationWarningKey{MutationWarningLocation: location, Message: message}
		if r.seen[key] {
			return nil
		}
		switch action {
		case "ignore":
			return nil
		case "error":
			return &MutationWarningError{message}
		case "default", "module", "once":
			if r.seen == nil {
				r.seen = make(map[mutationWarningKey]bool)
			}
			r.seen[key] = true
			if action != "default" {
				key.Line = 0
				key.AllLines = true
				if r.seen[key] {
					return nil
				}
				r.seen[key] = true
			}
		}
		if emit != nil {
			return emit(message)
		}
		return nil
	}
}
