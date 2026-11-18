// Package usageerr marks an error as a usage error, and as one whose message
// already says what to run, for the packages under internal/platform that
// return such errors and cannot import cmd/cliout, which recognizes the marks
// (cliout.IsUsage, and the help pointer Execute leaves off). It imports
// nothing but the standard library, so any package can depend on it.
//
// A type of a package's own marks itself the same way, by implementing Marker
// (and HelpOptOut): see deployment.NameError.
package usageerr

// Marker is an error that says it is a usage error: the command was invoked
// wrongly. cliout.IsUsage finds one anywhere in an error's chain.
type Marker interface{ IsUsageError() bool }

// HelpOptOut is a usage error that opts out of the pointer to the command's
// help, because its message already says what to run instead. cliout reads it
// from anywhere in an error's chain.
type HelpOptOut interface{ OmitsHelpPointer() bool }

// Usage marks err as a usage error: the command was invoked wrongly, such as a
// verb that does not fit the token it names. nil stays nil.
func Usage(err error) error {
	if err == nil {
		return nil
	}
	return usageError{err}
}

type usageError struct{ error }

func (e usageError) Unwrap() error    { return e.error }
func (usageError) IsUsageError() bool { return true }

// NoHelp is Usage for an error whose message already names what to run
// instead, such as the verb that fits the token: the pointer to the command's
// help would only bury it. nil stays nil.
func NoHelp(err error) error {
	if err == nil {
		return nil
	}
	return noHelpError{usageError{err}}
}

type noHelpError struct{ usageError }

func (noHelpError) OmitsHelpPointer() bool { return true }
