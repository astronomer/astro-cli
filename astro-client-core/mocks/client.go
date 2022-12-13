// Code generated by mockery v2.14.0. DO NOT EDIT.

package astrocore_mocks

import (
	context "context"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"

	io "io"

	mock "github.com/stretchr/testify/mock"
)

// ClientWithResponsesInterface is an autogenerated mock type for the ClientWithResponsesInterface type
type ClientWithResponsesInterface struct {
	mock.Mock
}

// CreateOrganizationWithBodyWithResponse provides a mock function with given fields: ctx, contentType, body, reqEditors
func (_m *ClientWithResponsesInterface) CreateOrganizationWithBodyWithResponse(ctx context.Context, contentType string, body io.Reader, reqEditors ...astrocore.RequestEditorFn) (*astrocore.CreateOrganizationResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, contentType, body)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astrocore.CreateOrganizationResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, io.Reader, ...astrocore.RequestEditorFn) *astrocore.CreateOrganizationResponse); ok {
		r0 = rf(ctx, contentType, body, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astrocore.CreateOrganizationResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, io.Reader, ...astrocore.RequestEditorFn) error); ok {
		r1 = rf(ctx, contentType, body, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// CreateOrganizationWithResponse provides a mock function with given fields: ctx, body, reqEditors
func (_m *ClientWithResponsesInterface) CreateOrganizationWithResponse(ctx context.Context, body astrocore.MutateOrganizationRequest, reqEditors ...astrocore.RequestEditorFn) (*astrocore.CreateOrganizationResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, body)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astrocore.CreateOrganizationResponse
	if rf, ok := ret.Get(0).(func(context.Context, astrocore.MutateOrganizationRequest, ...astrocore.RequestEditorFn) *astrocore.CreateOrganizationResponse); ok {
		r0 = rf(ctx, body, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astrocore.CreateOrganizationResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, astrocore.MutateOrganizationRequest, ...astrocore.RequestEditorFn) error); ok {
		r1 = rf(ctx, body, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// CreateSsoConnectionWithBodyWithResponse provides a mock function with given fields: ctx, orgShortNameId, contentType, body, reqEditors
func (_m *ClientWithResponsesInterface) CreateSsoConnectionWithBodyWithResponse(ctx context.Context, orgShortNameId string, contentType string, body io.Reader, reqEditors ...astrocore.RequestEditorFn) (*astrocore.CreateSsoConnectionResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, orgShortNameId, contentType, body)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astrocore.CreateSsoConnectionResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, io.Reader, ...astrocore.RequestEditorFn) *astrocore.CreateSsoConnectionResponse); ok {
		r0 = rf(ctx, orgShortNameId, contentType, body, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astrocore.CreateSsoConnectionResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, io.Reader, ...astrocore.RequestEditorFn) error); ok {
		r1 = rf(ctx, orgShortNameId, contentType, body, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// CreateSsoConnectionWithResponse provides a mock function with given fields: ctx, orgShortNameId, body, reqEditors
func (_m *ClientWithResponsesInterface) CreateSsoConnectionWithResponse(ctx context.Context, orgShortNameId string, body astrocore.CreateSsoConnectionRequest, reqEditors ...astrocore.RequestEditorFn) (*astrocore.CreateSsoConnectionResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, orgShortNameId, body)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astrocore.CreateSsoConnectionResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, astrocore.CreateSsoConnectionRequest, ...astrocore.RequestEditorFn) *astrocore.CreateSsoConnectionResponse); ok {
		r0 = rf(ctx, orgShortNameId, body, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astrocore.CreateSsoConnectionResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, astrocore.CreateSsoConnectionRequest, ...astrocore.RequestEditorFn) error); ok {
		r1 = rf(ctx, orgShortNameId, body, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// CreateUserInviteWithBodyWithResponse provides a mock function with given fields: ctx, orgShortNameId, contentType, body, reqEditors
func (_m *ClientWithResponsesInterface) CreateUserInviteWithBodyWithResponse(ctx context.Context, orgShortNameId string, contentType string, body io.Reader, reqEditors ...astrocore.RequestEditorFn) (*astrocore.CreateUserInviteResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, orgShortNameId, contentType, body)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astrocore.CreateUserInviteResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, io.Reader, ...astrocore.RequestEditorFn) *astrocore.CreateUserInviteResponse); ok {
		r0 = rf(ctx, orgShortNameId, contentType, body, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astrocore.CreateUserInviteResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, io.Reader, ...astrocore.RequestEditorFn) error); ok {
		r1 = rf(ctx, orgShortNameId, contentType, body, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// CreateUserInviteWithResponse provides a mock function with given fields: ctx, orgShortNameId, body, reqEditors
func (_m *ClientWithResponsesInterface) CreateUserInviteWithResponse(ctx context.Context, orgShortNameId string, body astrocore.CreateUserInviteRequest, reqEditors ...astrocore.RequestEditorFn) (*astrocore.CreateUserInviteResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, orgShortNameId, body)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astrocore.CreateUserInviteResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, astrocore.CreateUserInviteRequest, ...astrocore.RequestEditorFn) *astrocore.CreateUserInviteResponse); ok {
		r0 = rf(ctx, orgShortNameId, body, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astrocore.CreateUserInviteResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, astrocore.CreateUserInviteRequest, ...astrocore.RequestEditorFn) error); ok {
		r1 = rf(ctx, orgShortNameId, body, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// DeleteOrgUserWithResponse provides a mock function with given fields: ctx, orgShortNameId, userId, reqEditors
func (_m *ClientWithResponsesInterface) DeleteOrgUserWithResponse(ctx context.Context, orgShortNameId string, userId string, reqEditors ...astrocore.RequestEditorFn) (*astrocore.DeleteOrgUserResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, orgShortNameId, userId)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astrocore.DeleteOrgUserResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, ...astrocore.RequestEditorFn) *astrocore.DeleteOrgUserResponse); ok {
		r0 = rf(ctx, orgShortNameId, userId, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astrocore.DeleteOrgUserResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, ...astrocore.RequestEditorFn) error); ok {
		r1 = rf(ctx, orgShortNameId, userId, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// DeleteUserInviteWithResponse provides a mock function with given fields: ctx, orgShortNameId, inviteId, reqEditors
func (_m *ClientWithResponsesInterface) DeleteUserInviteWithResponse(ctx context.Context, orgShortNameId string, inviteId string, reqEditors ...astrocore.RequestEditorFn) (*astrocore.DeleteUserInviteResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, orgShortNameId, inviteId)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astrocore.DeleteUserInviteResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, ...astrocore.RequestEditorFn) *astrocore.DeleteUserInviteResponse); ok {
		r0 = rf(ctx, orgShortNameId, inviteId, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astrocore.DeleteUserInviteResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, ...astrocore.RequestEditorFn) error); ok {
		r1 = rf(ctx, orgShortNameId, inviteId, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// DeleteWorkspaceUserWithResponse provides a mock function with given fields: ctx, orgShortNameId, workspaceId, userId, reqEditors
func (_m *ClientWithResponsesInterface) DeleteWorkspaceUserWithResponse(ctx context.Context, orgShortNameId string, workspaceId string, userId string, reqEditors ...astrocore.RequestEditorFn) (*astrocore.DeleteWorkspaceUserResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, orgShortNameId, workspaceId, userId)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astrocore.DeleteWorkspaceUserResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, string, ...astrocore.RequestEditorFn) *astrocore.DeleteWorkspaceUserResponse); ok {
		r0 = rf(ctx, orgShortNameId, workspaceId, userId, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astrocore.DeleteWorkspaceUserResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, string, ...astrocore.RequestEditorFn) error); ok {
		r1 = rf(ctx, orgShortNameId, workspaceId, userId, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetManagedDomainWithResponse provides a mock function with given fields: ctx, orgShortNameId, domainId, reqEditors
func (_m *ClientWithResponsesInterface) GetManagedDomainWithResponse(ctx context.Context, orgShortNameId string, domainId string, reqEditors ...astrocore.RequestEditorFn) (*astrocore.GetManagedDomainResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, orgShortNameId, domainId)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astrocore.GetManagedDomainResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, ...astrocore.RequestEditorFn) *astrocore.GetManagedDomainResponse); ok {
		r0 = rf(ctx, orgShortNameId, domainId, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astrocore.GetManagedDomainResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, ...astrocore.RequestEditorFn) error); ok {
		r1 = rf(ctx, orgShortNameId, domainId, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetOrganizationAuditLogsWithResponse provides a mock function with given fields: ctx, orgShortNameId, params, reqEditors
func (_m *ClientWithResponsesInterface) GetOrganizationAuditLogsWithResponse(ctx context.Context, orgShortNameId string, params *astrocore.GetOrganizationAuditLogsParams, reqEditors ...astrocore.RequestEditorFn) (*astrocore.GetOrganizationAuditLogsResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, orgShortNameId, params)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astrocore.GetOrganizationAuditLogsResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, *astrocore.GetOrganizationAuditLogsParams, ...astrocore.RequestEditorFn) *astrocore.GetOrganizationAuditLogsResponse); ok {
		r0 = rf(ctx, orgShortNameId, params, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astrocore.GetOrganizationAuditLogsResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, *astrocore.GetOrganizationAuditLogsParams, ...astrocore.RequestEditorFn) error); ok {
		r1 = rf(ctx, orgShortNameId, params, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetOrganizationWithResponse provides a mock function with given fields: ctx, orgShortNameId, reqEditors
func (_m *ClientWithResponsesInterface) GetOrganizationWithResponse(ctx context.Context, orgShortNameId string, reqEditors ...astrocore.RequestEditorFn) (*astrocore.GetOrganizationResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, orgShortNameId)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astrocore.GetOrganizationResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, ...astrocore.RequestEditorFn) *astrocore.GetOrganizationResponse); ok {
		r0 = rf(ctx, orgShortNameId, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astrocore.GetOrganizationResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, ...astrocore.RequestEditorFn) error); ok {
		r1 = rf(ctx, orgShortNameId, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetSelfUserWithResponse provides a mock function with given fields: ctx, params, reqEditors
func (_m *ClientWithResponsesInterface) GetSelfUserWithResponse(ctx context.Context, params *astrocore.GetSelfUserParams, reqEditors ...astrocore.RequestEditorFn) (*astrocore.GetSelfUserResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, params)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astrocore.GetSelfUserResponse
	if rf, ok := ret.Get(0).(func(context.Context, *astrocore.GetSelfUserParams, ...astrocore.RequestEditorFn) *astrocore.GetSelfUserResponse); ok {
		r0 = rf(ctx, params, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astrocore.GetSelfUserResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, *astrocore.GetSelfUserParams, ...astrocore.RequestEditorFn) error); ok {
		r1 = rf(ctx, params, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetSsoConnectionWithResponse provides a mock function with given fields: ctx, orgShortNameId, connectionId, reqEditors
func (_m *ClientWithResponsesInterface) GetSsoConnectionWithResponse(ctx context.Context, orgShortNameId string, connectionId string, reqEditors ...astrocore.RequestEditorFn) (*astrocore.GetSsoConnectionResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, orgShortNameId, connectionId)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astrocore.GetSsoConnectionResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, ...astrocore.RequestEditorFn) *astrocore.GetSsoConnectionResponse); ok {
		r0 = rf(ctx, orgShortNameId, connectionId, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astrocore.GetSsoConnectionResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, ...astrocore.RequestEditorFn) error); ok {
		r1 = rf(ctx, orgShortNameId, connectionId, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetUserWithResponse provides a mock function with given fields: ctx, orgShortNameId, userId, reqEditors
func (_m *ClientWithResponsesInterface) GetUserWithResponse(ctx context.Context, orgShortNameId string, userId string, reqEditors ...astrocore.RequestEditorFn) (*astrocore.GetUserResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, orgShortNameId, userId)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astrocore.GetUserResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, ...astrocore.RequestEditorFn) *astrocore.GetUserResponse); ok {
		r0 = rf(ctx, orgShortNameId, userId, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astrocore.GetUserResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, ...astrocore.RequestEditorFn) error); ok {
		r1 = rf(ctx, orgShortNameId, userId, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ListManagedDomainsWithResponse provides a mock function with given fields: ctx, orgShortNameId, reqEditors
func (_m *ClientWithResponsesInterface) ListManagedDomainsWithResponse(ctx context.Context, orgShortNameId string, reqEditors ...astrocore.RequestEditorFn) (*astrocore.ListManagedDomainsResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, orgShortNameId)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astrocore.ListManagedDomainsResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, ...astrocore.RequestEditorFn) *astrocore.ListManagedDomainsResponse); ok {
		r0 = rf(ctx, orgShortNameId, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astrocore.ListManagedDomainsResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, ...astrocore.RequestEditorFn) error); ok {
		r1 = rf(ctx, orgShortNameId, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ListOrgUsersWithResponse provides a mock function with given fields: ctx, orgShortNameId, params, reqEditors
func (_m *ClientWithResponsesInterface) ListOrgUsersWithResponse(ctx context.Context, orgShortNameId string, params *astrocore.ListOrgUsersParams, reqEditors ...astrocore.RequestEditorFn) (*astrocore.ListOrgUsersResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, orgShortNameId, params)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astrocore.ListOrgUsersResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, *astrocore.ListOrgUsersParams, ...astrocore.RequestEditorFn) *astrocore.ListOrgUsersResponse); ok {
		r0 = rf(ctx, orgShortNameId, params, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astrocore.ListOrgUsersResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, *astrocore.ListOrgUsersParams, ...astrocore.RequestEditorFn) error); ok {
		r1 = rf(ctx, orgShortNameId, params, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ListOrganizationAuthIdsWithResponse provides a mock function with given fields: ctx, params, reqEditors
func (_m *ClientWithResponsesInterface) ListOrganizationAuthIdsWithResponse(ctx context.Context, params *astrocore.ListOrganizationAuthIdsParams, reqEditors ...astrocore.RequestEditorFn) (*astrocore.ListOrganizationAuthIdsResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, params)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astrocore.ListOrganizationAuthIdsResponse
	if rf, ok := ret.Get(0).(func(context.Context, *astrocore.ListOrganizationAuthIdsParams, ...astrocore.RequestEditorFn) *astrocore.ListOrganizationAuthIdsResponse); ok {
		r0 = rf(ctx, params, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astrocore.ListOrganizationAuthIdsResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, *astrocore.ListOrganizationAuthIdsParams, ...astrocore.RequestEditorFn) error); ok {
		r1 = rf(ctx, params, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ListOrganizationsWithResponse provides a mock function with given fields: ctx, reqEditors
func (_m *ClientWithResponsesInterface) ListOrganizationsWithResponse(ctx context.Context, reqEditors ...astrocore.RequestEditorFn) (*astrocore.ListOrganizationsResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astrocore.ListOrganizationsResponse
	if rf, ok := ret.Get(0).(func(context.Context, ...astrocore.RequestEditorFn) *astrocore.ListOrganizationsResponse); ok {
		r0 = rf(ctx, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astrocore.ListOrganizationsResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, ...astrocore.RequestEditorFn) error); ok {
		r1 = rf(ctx, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ListSsoConnectionsWithResponse provides a mock function with given fields: ctx, orgShortNameId, reqEditors
func (_m *ClientWithResponsesInterface) ListSsoConnectionsWithResponse(ctx context.Context, orgShortNameId string, reqEditors ...astrocore.RequestEditorFn) (*astrocore.ListSsoConnectionsResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, orgShortNameId)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astrocore.ListSsoConnectionsResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, ...astrocore.RequestEditorFn) *astrocore.ListSsoConnectionsResponse); ok {
		r0 = rf(ctx, orgShortNameId, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astrocore.ListSsoConnectionsResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, ...astrocore.RequestEditorFn) error); ok {
		r1 = rf(ctx, orgShortNameId, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ListWorkspaceUsersWithResponse provides a mock function with given fields: ctx, orgShortNameId, workspaceId, params, reqEditors
func (_m *ClientWithResponsesInterface) ListWorkspaceUsersWithResponse(ctx context.Context, orgShortNameId string, workspaceId string, params *astrocore.ListWorkspaceUsersParams, reqEditors ...astrocore.RequestEditorFn) (*astrocore.ListWorkspaceUsersResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, orgShortNameId, workspaceId, params)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astrocore.ListWorkspaceUsersResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, *astrocore.ListWorkspaceUsersParams, ...astrocore.RequestEditorFn) *astrocore.ListWorkspaceUsersResponse); ok {
		r0 = rf(ctx, orgShortNameId, workspaceId, params, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astrocore.ListWorkspaceUsersResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, *astrocore.ListWorkspaceUsersParams, ...astrocore.RequestEditorFn) error); ok {
		r1 = rf(ctx, orgShortNameId, workspaceId, params, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MutateOrgUserRoleWithBodyWithResponse provides a mock function with given fields: ctx, orgShortNameId, userId, contentType, body, reqEditors
func (_m *ClientWithResponsesInterface) MutateOrgUserRoleWithBodyWithResponse(ctx context.Context, orgShortNameId string, userId string, contentType string, body io.Reader, reqEditors ...astrocore.RequestEditorFn) (*astrocore.MutateOrgUserRoleResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, orgShortNameId, userId, contentType, body)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astrocore.MutateOrgUserRoleResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, string, io.Reader, ...astrocore.RequestEditorFn) *astrocore.MutateOrgUserRoleResponse); ok {
		r0 = rf(ctx, orgShortNameId, userId, contentType, body, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astrocore.MutateOrgUserRoleResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, string, io.Reader, ...astrocore.RequestEditorFn) error); ok {
		r1 = rf(ctx, orgShortNameId, userId, contentType, body, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MutateOrgUserRoleWithResponse provides a mock function with given fields: ctx, orgShortNameId, userId, body, reqEditors
func (_m *ClientWithResponsesInterface) MutateOrgUserRoleWithResponse(ctx context.Context, orgShortNameId string, userId string, body astrocore.MutateOrgUserRoleRequest, reqEditors ...astrocore.RequestEditorFn) (*astrocore.MutateOrgUserRoleResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, orgShortNameId, userId, body)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astrocore.MutateOrgUserRoleResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, astrocore.MutateOrgUserRoleRequest, ...astrocore.RequestEditorFn) *astrocore.MutateOrgUserRoleResponse); ok {
		r0 = rf(ctx, orgShortNameId, userId, body, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astrocore.MutateOrgUserRoleResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, astrocore.MutateOrgUserRoleRequest, ...astrocore.RequestEditorFn) error); ok {
		r1 = rf(ctx, orgShortNameId, userId, body, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MutateWorkspaceUserRoleWithBodyWithResponse provides a mock function with given fields: ctx, orgShortNameId, workspaceId, userId, contentType, body, reqEditors
func (_m *ClientWithResponsesInterface) MutateWorkspaceUserRoleWithBodyWithResponse(ctx context.Context, orgShortNameId string, workspaceId string, userId string, contentType string, body io.Reader, reqEditors ...astrocore.RequestEditorFn) (*astrocore.MutateWorkspaceUserRoleResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, orgShortNameId, workspaceId, userId, contentType, body)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astrocore.MutateWorkspaceUserRoleResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, string, string, io.Reader, ...astrocore.RequestEditorFn) *astrocore.MutateWorkspaceUserRoleResponse); ok {
		r0 = rf(ctx, orgShortNameId, workspaceId, userId, contentType, body, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astrocore.MutateWorkspaceUserRoleResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, string, string, io.Reader, ...astrocore.RequestEditorFn) error); ok {
		r1 = rf(ctx, orgShortNameId, workspaceId, userId, contentType, body, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MutateWorkspaceUserRoleWithResponse provides a mock function with given fields: ctx, orgShortNameId, workspaceId, userId, body, reqEditors
func (_m *ClientWithResponsesInterface) MutateWorkspaceUserRoleWithResponse(ctx context.Context, orgShortNameId string, workspaceId string, userId string, body astrocore.MutateWorkspaceUserRoleRequest, reqEditors ...astrocore.RequestEditorFn) (*astrocore.MutateWorkspaceUserRoleResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, orgShortNameId, workspaceId, userId, body)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astrocore.MutateWorkspaceUserRoleResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, string, astrocore.MutateWorkspaceUserRoleRequest, ...astrocore.RequestEditorFn) *astrocore.MutateWorkspaceUserRoleResponse); ok {
		r0 = rf(ctx, orgShortNameId, workspaceId, userId, body, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astrocore.MutateWorkspaceUserRoleResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, string, astrocore.MutateWorkspaceUserRoleRequest, ...astrocore.RequestEditorFn) error); ok {
		r1 = rf(ctx, orgShortNameId, workspaceId, userId, body, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// UpdateOrganizationWithBodyWithResponse provides a mock function with given fields: ctx, orgShortNameId, contentType, body, reqEditors
func (_m *ClientWithResponsesInterface) UpdateOrganizationWithBodyWithResponse(ctx context.Context, orgShortNameId string, contentType string, body io.Reader, reqEditors ...astrocore.RequestEditorFn) (*astrocore.UpdateOrganizationResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, orgShortNameId, contentType, body)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astrocore.UpdateOrganizationResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, io.Reader, ...astrocore.RequestEditorFn) *astrocore.UpdateOrganizationResponse); ok {
		r0 = rf(ctx, orgShortNameId, contentType, body, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astrocore.UpdateOrganizationResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, io.Reader, ...astrocore.RequestEditorFn) error); ok {
		r1 = rf(ctx, orgShortNameId, contentType, body, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// UpdateOrganizationWithResponse provides a mock function with given fields: ctx, orgShortNameId, body, reqEditors
func (_m *ClientWithResponsesInterface) UpdateOrganizationWithResponse(ctx context.Context, orgShortNameId string, body astrocore.MutateOrganizationRequest, reqEditors ...astrocore.RequestEditorFn) (*astrocore.UpdateOrganizationResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, orgShortNameId, body)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astrocore.UpdateOrganizationResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, astrocore.MutateOrganizationRequest, ...astrocore.RequestEditorFn) *astrocore.UpdateOrganizationResponse); ok {
		r0 = rf(ctx, orgShortNameId, body, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astrocore.UpdateOrganizationResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, astrocore.MutateOrganizationRequest, ...astrocore.RequestEditorFn) error); ok {
		r1 = rf(ctx, orgShortNameId, body, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// UpdateSelfUserInviteWithBodyWithResponse provides a mock function with given fields: ctx, inviteId, contentType, body, reqEditors
func (_m *ClientWithResponsesInterface) UpdateSelfUserInviteWithBodyWithResponse(ctx context.Context, inviteId string, contentType string, body io.Reader, reqEditors ...astrocore.RequestEditorFn) (*astrocore.UpdateSelfUserInviteResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, inviteId, contentType, body)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astrocore.UpdateSelfUserInviteResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, io.Reader, ...astrocore.RequestEditorFn) *astrocore.UpdateSelfUserInviteResponse); ok {
		r0 = rf(ctx, inviteId, contentType, body, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astrocore.UpdateSelfUserInviteResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, io.Reader, ...astrocore.RequestEditorFn) error); ok {
		r1 = rf(ctx, inviteId, contentType, body, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// UpdateSelfUserInviteWithResponse provides a mock function with given fields: ctx, inviteId, body, reqEditors
func (_m *ClientWithResponsesInterface) UpdateSelfUserInviteWithResponse(ctx context.Context, inviteId string, body astrocore.UpdateInviteRequest, reqEditors ...astrocore.RequestEditorFn) (*astrocore.UpdateSelfUserInviteResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, inviteId, body)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astrocore.UpdateSelfUserInviteResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, astrocore.UpdateInviteRequest, ...astrocore.RequestEditorFn) *astrocore.UpdateSelfUserInviteResponse); ok {
		r0 = rf(ctx, inviteId, body, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astrocore.UpdateSelfUserInviteResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, astrocore.UpdateInviteRequest, ...astrocore.RequestEditorFn) error); ok {
		r1 = rf(ctx, inviteId, body, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

type mockConstructorTestingTNewClientWithResponsesInterface interface {
	mock.TestingT
	Cleanup(func())
}

// NewClientWithResponsesInterface creates a new instance of ClientWithResponsesInterface. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewClientWithResponsesInterface(t mockConstructorTestingTNewClientWithResponsesInterface) *ClientWithResponsesInterface {
	mock := &ClientWithResponsesInterface{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
