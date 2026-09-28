package airflowapi

import (
	"context"
	"encoding/json"
	"net/url"
	"sort"
	"strconv"
)

// Provider is one installed provider distribution.
type Provider struct {
	PackageName string `json:"package_name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	// DocumentationURL is Airflow 3 only.
	DocumentationURL string `json:"documentation_url,omitempty"`
}

// ProviderList is every provider an Airflow has installed.
type ProviderList struct {
	Providers    []Provider `json:"providers"`
	TotalEntries int        `json:"total_entries"`
}

// providerPageCap bounds how many pages ListProviders reads, so a server that
// keeps reporting more than it sends cannot hold the loop forever.
const providerPageCap = 50

// ListProviders lists every installed provider.
//
// The generations page this endpoint differently. Airflow 2 takes no
// parameters and answers with the whole set; Airflow 3 pages it, defaulting to
// fifty, which a Runtime image with its usual providers already exceeds. So
// Airflow 3 is read page by page until the total it reports has arrived, and
// the caller always gets the whole list — a provider missing from a listing
// someone reads to check an installed version is a wrong answer, not a short
// one.
func (c *Client) ListProviders(ctx context.Context) (ProviderList, error) {
	generation, err := c.Generation(ctx)
	if err != nil {
		return ProviderList{}, err
	}
	if generation != Airflow3 {
		var list ProviderList
		err := c.getCollection(ctx, "/providers", nil, &list)
		return list, err
	}
	var all ProviderList
	for page := 0; page < providerPageCap; page++ {
		var list ProviderList
		query := ListOptions{Offset: len(all.Providers)}.query()
		if err := c.getCollection(ctx, "/providers", query, &list); err != nil {
			return ProviderList{}, err
		}
		all.Providers = append(all.Providers, list.Providers...)
		all.TotalEntries = list.TotalEntries
		if len(list.Providers) == 0 || len(all.Providers) >= list.TotalEntries {
			break
		}
	}
	return all, nil
}

// Plugin is one Airflow plugin: its name, where it was loaded from, and what
// it contributes.
type Plugin struct {
	Name   string
	Source string
	// Components maps each kind of thing the plugin contributes, under the
	// field name Airflow gives it ("macros", "fastapi_apps", ...), to the
	// names of what it contributes there. Kinds it contributes nothing to are
	// left out.
	//
	// A map rather than fields because the two generations list different
	// kinds — Airflow 3 dropped hooks and executors and added FastAPI and
	// React apps — and a field per kind would be a list of what one release
	// had. Object entries (an app, a view) are named by their "name".
	Components map[string][]string
}

// PluginList is a page of plugins.
type PluginList struct {
	Plugins      []Plugin
	TotalEntries int
}

// ListPlugins lists the plugins an Airflow loaded.
func (c *Client) ListPlugins(ctx context.Context, opts ListOptions) (PluginList, error) {
	var wire struct {
		Plugins      []map[string]json.RawMessage `json:"plugins"`
		TotalEntries int                          `json:"total_entries"`
	}
	if err := c.getCollection(ctx, "/plugins", opts.query(), &wire); err != nil {
		return PluginList{}, err
	}
	list := PluginList{TotalEntries: wire.TotalEntries, Plugins: make([]Plugin, 0, len(wire.Plugins))}
	for _, fields := range wire.Plugins {
		list.Plugins = append(list.Plugins, pluginOf(fields))
	}
	return list, nil
}

func pluginOf(fields map[string]json.RawMessage) Plugin {
	plugin := Plugin{Components: map[string][]string{}}
	for key, raw := range fields {
		switch key {
		case "name":
			plugin.Name = decodeString(raw)
		case "source":
			plugin.Source = decodeString(raw)
		default:
			if names := componentNames(raw); len(names) > 0 {
				plugin.Components[key] = names
			}
		}
	}
	return plugin
}

// decodeString reads a JSON string, treating null or any other shape as "".
func decodeString(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

// componentNames reads one of a plugin's component lists. Airflow sends most
// as strings and some as objects; an object is named by its "name", and one
// without a name by its JSON so it is not dropped. A value that is not a list
// is not a component and yields nothing.
func componentNames(raw json.RawMessage) []string {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		var s string
		if err := json.Unmarshal(item, &s); err == nil {
			if s != "" {
				names = append(names, s)
			}
			continue
		}
		var named struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(item, &named); err == nil && named.Name != "" {
			names = append(names, named.Name)
			continue
		}
		if string(item) != jsonNull {
			names = append(names, string(item))
		}
	}
	sort.Strings(names)
	return names
}

// ConfigOption is one configuration value.
type ConfigOption struct {
	Section string
	Key     string
	Value   string
	// Source is where the value came from ("airflow.cfg", "env var",
	// "default"), when Airflow says. Only Airflow 3 does, and only when asked
	// to display sources.
	Source string
}

// Config is an Airflow's configuration, flattened to one entry per option in
// the order Airflow listed them.
type Config struct {
	Options []ConfigOption
}

// Config reads an Airflow's configuration, or one section of it.
//
// Both generations refuse this with a 403 unless the deployment exposes its
// configuration — [webserver] expose_config on Airflow 2, [api] expose_config
// on Airflow 3 — and both default to not exposing it. errors.Is(err,
// ErrForbidden) is that answer.
func (c *Client) Config(ctx context.Context, section string) (Config, error) {
	query := url.Values{}
	if section != "" {
		query.Set("section", section)
	}
	var wire struct {
		Sections []struct {
			Name    string `json:"name"`
			Options []struct {
				Key   string          `json:"key"`
				Value json.RawMessage `json:"value"`
			} `json:"options"`
		} `json:"sections"`
	}
	if err := c.get(ctx, "/config", query, &wire); err != nil {
		return Config{}, err
	}
	var config Config
	for _, s := range wire.Sections {
		for _, o := range s.Options {
			value, source := configValue(o.Value)
			config.Options = append(config.Options, ConfigOption{Section: s.Name, Key: o.Key, Value: value, Source: source})
		}
	}
	return config, nil
}

// configValue reads an option's value. Airflow 2 sends a string. Airflow 3
// sends a string, or a [value, source] pair when sources are displayed, and a
// masked sensitive value arrives the same way. Anything else is kept as its
// JSON rather than dropped.
func configValue(raw json.RawMessage) (value, source string) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, ""
	}
	var pair []json.RawMessage
	if err := json.Unmarshal(raw, &pair); err == nil && len(pair) == 2 {
		return scalarText(pair[0]), scalarText(pair[1])
	}
	if string(raw) == jsonNull {
		return "", ""
	}
	return string(raw), ""
}

// scalarText renders a JSON scalar as text: a string unquoted, a number or a
// boolean as written.
func scalarText(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	if f, err := strconv.ParseFloat(string(raw), 64); err == nil {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	if string(raw) == jsonNull {
		return ""
	}
	return string(raw)
}

// jsonNull is JSON's null, as a raw value spells it.
const jsonNull = "null"
