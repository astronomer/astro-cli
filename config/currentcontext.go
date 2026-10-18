package config

// CurrentContextWithoutLogin is the current context as GetCurrentContext
// reads it, with the same errors (ErrGetHomeString when no context is
// current, ErrNotConnected when the config holds none for the current
// domain) and an environment login's organization and workspace over the
// config's, but with no login: Token and RefreshToken are empty.
//
// Unlike GetCurrentContext it never resolves the login (resolveLogin), so it
// reads no keyring or vault, rewrites no config and prints nothing. It is for
// what the CLI decides on every command, before it knows which one runs:
// which platform the context is on (context.IsCloudContext), and whether the
// organization is hosted, which some Astro commands' flags depend on
// (organization.IsOrgHosted).
func CurrentContextWithoutLogin() (Context, error) {
	c := Context{}
	var err error
	if c.Domain, err = GetCurrentDomain(); err != nil {
		return Context{}, err
	}
	key, err := c.GetContextKey()
	if err != nil {
		return Context{}, err
	}
	if !c.ContextExists() {
		return Context{}, ErrNotConnected
	}
	if err := viperHome.UnmarshalKey(contextsKey+"."+key, &c); err != nil {
		return Context{}, err
	}
	if l, ok := envLogin(key); ok {
		applyEnvironmentLogin(&l, &c)
	}
	c.Token, c.RefreshToken = "", ""
	return c, nil
}
