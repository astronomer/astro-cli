package deploy

// BeforeAsking is d with before called each time it is about to ask a
// question (ConfirmTarget, ResolveUnlinked), so a command holding its
// warnings for the end shows the ones found so far where they bear on the
// answer.
func BeforeAsking(d Deployer, before func()) Deployer {
	return beforeAsking{Deployer: d, before: before}
}

type beforeAsking struct {
	Deployer
	before func()
}

func (b beforeAsking) ConfirmTarget(choices []Choice, defaultLink string) (string, error) {
	b.before()
	return b.Deployer.ConfirmTarget(choices, defaultLink)
}

func (b beforeAsking) ResolveUnlinked(workspaceID string) (Deployment, error) {
	b.before()
	return b.Deployer.ResolveUnlinked(workspaceID)
}
