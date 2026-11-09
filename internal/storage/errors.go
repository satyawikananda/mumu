package storage

import "errors"

// Sentinel errors let callers show an actionable message without parsing
// driver text. Existing data is never deleted to recover from them.
var (
	// ErrCorrupt reports a database file SQLite cannot read.
	ErrCorrupt = errors.New("local library is corrupt")
	// ErrSchemaNewer reports a database written by a newer mumu build.
	ErrSchemaNewer = errors.New("local library schema is newer than this app")
	// ErrUnknownSchema reports tables without version metadata.
	ErrUnknownSchema = errors.New("local library schema is not recognized")
	// ErrPermission reports a database path that cannot be read or written.
	ErrPermission = errors.New("local library is not writable")
	// ErrCredentialsUnsupported reports a source URL with user information.
	ErrCredentialsUnsupported = errors.New("source credentials are not supported yet")
	// ErrNotFound reports a missing record.
	ErrNotFound = errors.New("record not found")
	// ErrDuplicate reports a uniqueness conflict.
	ErrDuplicate = errors.New("record already exists")
	// ErrInvalid reports a record that cannot be stored.
	ErrInvalid = errors.New("invalid storage record")
)
