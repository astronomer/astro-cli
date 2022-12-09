package types

type ValuesConfig struct {
	AcmCertArn               string
	Airgapped                bool
	Aws                      bool
	Azure                    bool
	BaseDomain               string
	EmailNoReply             string
	EnableEmail              bool
	Gcloud                   bool
	EnableManualReleaseNames bool
	DisableNginx             bool
	PrivateCA                bool
	PrivateLoadBalancer      bool
	PrivateRegistryRepo      string
	EnablePublicSignups      bool
}
