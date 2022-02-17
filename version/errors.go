package version

type ErrVersionMismatch struct{}

func (e ErrVersionMismatch) Error() string {
	return `
  Some fields requested by the CLI are not available in the server schema, it seems there is a version mismatch between your CLI and the server.
  In order to fix this issue, you might want to use the same minor version for both the server and the CLI.
  You can use the command "astro version" to check your CLI and server versions.
  You can use the flag --verbosity=debug to get the full error trace.
	`
}
