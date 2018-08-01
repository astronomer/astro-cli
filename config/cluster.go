package config

import (
	"errors"
	"fmt"
	"strings"
)

type Clusters struct {
	Clusters map[string]Cluster `mapstructure:"clusters"`
}

type Cluster struct {
	Domain    string `mapstructure:"domain"`
	Workspace string `mapstructure:"workspace"`
	Token     string `mapstructure:"token"`
}

// scrubClusterKey allows a cluster domain to be used without interfering
// with viper's dot (.) notation for fetching configs by replacing with underscores (_)
func (c Cluster) GetClusterKey() (string, error) {
	if len(c.Domain) == 0 {
		return "", errors.New("Cluster config invalid, no domain specified")
	}

	return strings.Replace(c.Domain, ".", "_", -1), nil
}

// ClusterExists
func ClusterExists(domain string) bool {
	c := Cluster{Domain: domain}
	return c.ClusterExists()
}

func (c Cluster) ClusterExists() bool {
	key, err := c.GetClusterKey()
	if err != nil {
		return false
	}

	return viperHome.IsSet("clusters" + "." + key)
}

// GetCluster
func GetCluster(domain string) (Cluster, error) {
	c := Cluster{Domain: domain}
	return c.GetCluster()
}

func (c Cluster) GetCluster() (Cluster, error) {
	key, err := c.GetClusterKey()
	if err != nil {
		return c, err
	}

	if !c.ClusterExists() {
		return c, errors.New("Cluster not set, have you authenticated to this cluster?")
	}

	err = viperHome.UnmarshalKey("clusters"+"."+key, &c)
	if err != nil {
		return c, err
	}
	return c, nil
}

func GetCurrentCluster() (Cluster, error) {
	c := Cluster{}
	domain := viperHome.GetString("context")
	if len(domain) == 0 {
		return Cluster{}, errors.New("No context set, have you authenticated to a cluster?")
	}

	c.Domain = domain
	return c.GetCluster()
}

// GetClusters
func GetClusters() (Clusters, error) {
	var C Clusters
	err := viperHome.Unmarshal(&C)
	if err != nil {
		return C, err
	}

	return C, nil
}

// SetCluster
func (c Cluster) SetCluster() error {
	key, err := c.GetClusterKey()
	if err != nil {
		return err
	}

	cluster := map[string]string{
		"token":     c.Token,
		"domain":    c.Domain,
		"workspace": c.Workspace,
	}

	viperHome.Set("clusters"+"."+key, cluster)
	saveConfig(viperHome, HomeConfigFile)

	return nil
}

func (c Cluster) SetClusterKey(key, value string) error {
	cKey, err := c.GetClusterKey()
	if err != nil {
		return err
	}

	cfgPath := fmt.Sprintf("clusters.%s.%s", cKey, key)
	viperHome.Set(cfgPath, value)
	saveConfig(viperHome, HomeConfigFile)

	return nil
}

func (c Cluster) SwitchCluster() error {
	if c.ClusterExists() {
		viperHome.Set("context", c.Domain)
		saveConfig(viperHome, HomeConfigFile)
	}

	return nil
}

func (c Cluster) GetAPIURL() string {
	return fmt.Sprintf(
		"%s://houston.%s:%s/v1",
		CFG.CloudAPIProtocol.GetString(),
		c.Domain,
		CFG.CloudAPIPort.GetString(),
	)
}
