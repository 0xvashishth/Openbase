package metadata

import "errors"

// IsConflict reports whether err wraps ErrConflict.
func IsConflict(err error) bool {
	return errors.Is(err, ErrConflict)
}
