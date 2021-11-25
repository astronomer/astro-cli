package podman

import (
	"net/url"
)

// ToParams formats struct fields to be passed to API service
func (o *PushOptions) ToParams() (url.Values, error) {
	return ToParams(o)
}

// GetSkipTLSVerify returns value of field SkipTLSVerify
func (o *PushOptions) GetSkipTLSVerify() bool {
	if o.SkipTLSVerify == nil {
		var z bool
		return z
	}
	return *o.SkipTLSVerify
}

// GetIdentityToken returns value of field IdentityToken
func (o *PushOptions) GetIdentityToken() string {
	if o.IdentityToken == nil {
		var z string
		return z
	}
	return *o.IdentityToken
}

// WithIdentityToken set field IdentityToken to given value
func (o *PushOptions) WithIdentityToken(token string) *PushOptions {
	o.IdentityToken = &token
	return o
}
