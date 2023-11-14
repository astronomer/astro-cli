// Code generated by mockery v2.14.0. DO NOT EDIT.

package astroiamcore_mocks

import (
	context "context"

	astroiamcore "github.com/astronomer/astro-cli/astro-client-iam-core"

	io "io"

	mock "github.com/stretchr/testify/mock"
)

// ClientWithResponsesInterface is an autogenerated mock type for the ClientWithResponsesInterface type
type ClientWithResponsesInterface struct {
	mock.Mock
}

// AddTeamMembersWithBodyWithResponse provides a mock function with given fields: ctx, organizationId, teamId, contentType, body, reqEditors
func (_m *ClientWithResponsesInterface) AddTeamMembersWithBodyWithResponse(ctx context.Context, organizationId string, teamId string, contentType string, body io.Reader, reqEditors ...astroiamcore.RequestEditorFn) (*astroiamcore.AddTeamMembersResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, organizationId, teamId, contentType, body)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astroiamcore.AddTeamMembersResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, string, io.Reader, ...astroiamcore.RequestEditorFn) *astroiamcore.AddTeamMembersResponse); ok {
		r0 = rf(ctx, organizationId, teamId, contentType, body, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astroiamcore.AddTeamMembersResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, string, io.Reader, ...astroiamcore.RequestEditorFn) error); ok {
		r1 = rf(ctx, organizationId, teamId, contentType, body, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// AddTeamMembersWithResponse provides a mock function with given fields: ctx, organizationId, teamId, body, reqEditors
func (_m *ClientWithResponsesInterface) AddTeamMembersWithResponse(ctx context.Context, organizationId string, teamId string, body astroiamcore.AddTeamMembersRequest, reqEditors ...astroiamcore.RequestEditorFn) (*astroiamcore.AddTeamMembersResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, organizationId, teamId, body)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astroiamcore.AddTeamMembersResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, astroiamcore.AddTeamMembersRequest, ...astroiamcore.RequestEditorFn) *astroiamcore.AddTeamMembersResponse); ok {
		r0 = rf(ctx, organizationId, teamId, body, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astroiamcore.AddTeamMembersResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, astroiamcore.AddTeamMembersRequest, ...astroiamcore.RequestEditorFn) error); ok {
		r1 = rf(ctx, organizationId, teamId, body, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// CreateApiTokenWithBodyWithResponse provides a mock function with given fields: ctx, organizationId, contentType, body, reqEditors
func (_m *ClientWithResponsesInterface) CreateApiTokenWithBodyWithResponse(ctx context.Context, organizationId string, contentType string, body io.Reader, reqEditors ...astroiamcore.RequestEditorFn) (*astroiamcore.CreateApiTokenResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, organizationId, contentType, body)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astroiamcore.CreateApiTokenResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, io.Reader, ...astroiamcore.RequestEditorFn) *astroiamcore.CreateApiTokenResponse); ok {
		r0 = rf(ctx, organizationId, contentType, body, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astroiamcore.CreateApiTokenResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, io.Reader, ...astroiamcore.RequestEditorFn) error); ok {
		r1 = rf(ctx, organizationId, contentType, body, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// CreateApiTokenWithResponse provides a mock function with given fields: ctx, organizationId, body, reqEditors
func (_m *ClientWithResponsesInterface) CreateApiTokenWithResponse(ctx context.Context, organizationId string, body astroiamcore.CreateApiTokenRequest, reqEditors ...astroiamcore.RequestEditorFn) (*astroiamcore.CreateApiTokenResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, organizationId, body)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astroiamcore.CreateApiTokenResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, astroiamcore.CreateApiTokenRequest, ...astroiamcore.RequestEditorFn) *astroiamcore.CreateApiTokenResponse); ok {
		r0 = rf(ctx, organizationId, body, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astroiamcore.CreateApiTokenResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, astroiamcore.CreateApiTokenRequest, ...astroiamcore.RequestEditorFn) error); ok {
		r1 = rf(ctx, organizationId, body, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// CreateTeamWithBodyWithResponse provides a mock function with given fields: ctx, organizationId, contentType, body, reqEditors
func (_m *ClientWithResponsesInterface) CreateTeamWithBodyWithResponse(ctx context.Context, organizationId string, contentType string, body io.Reader, reqEditors ...astroiamcore.RequestEditorFn) (*astroiamcore.CreateTeamResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, organizationId, contentType, body)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astroiamcore.CreateTeamResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, io.Reader, ...astroiamcore.RequestEditorFn) *astroiamcore.CreateTeamResponse); ok {
		r0 = rf(ctx, organizationId, contentType, body, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astroiamcore.CreateTeamResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, io.Reader, ...astroiamcore.RequestEditorFn) error); ok {
		r1 = rf(ctx, organizationId, contentType, body, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// CreateTeamWithResponse provides a mock function with given fields: ctx, organizationId, body, reqEditors
func (_m *ClientWithResponsesInterface) CreateTeamWithResponse(ctx context.Context, organizationId string, body astroiamcore.CreateTeamRequest, reqEditors ...astroiamcore.RequestEditorFn) (*astroiamcore.CreateTeamResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, organizationId, body)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astroiamcore.CreateTeamResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, astroiamcore.CreateTeamRequest, ...astroiamcore.RequestEditorFn) *astroiamcore.CreateTeamResponse); ok {
		r0 = rf(ctx, organizationId, body, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astroiamcore.CreateTeamResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, astroiamcore.CreateTeamRequest, ...astroiamcore.RequestEditorFn) error); ok {
		r1 = rf(ctx, organizationId, body, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// CreateUserInviteWithBodyWithResponse provides a mock function with given fields: ctx, organizationId, contentType, body, reqEditors
func (_m *ClientWithResponsesInterface) CreateUserInviteWithBodyWithResponse(ctx context.Context, organizationId string, contentType string, body io.Reader, reqEditors ...astroiamcore.RequestEditorFn) (*astroiamcore.CreateUserInviteResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, organizationId, contentType, body)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astroiamcore.CreateUserInviteResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, io.Reader, ...astroiamcore.RequestEditorFn) *astroiamcore.CreateUserInviteResponse); ok {
		r0 = rf(ctx, organizationId, contentType, body, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astroiamcore.CreateUserInviteResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, io.Reader, ...astroiamcore.RequestEditorFn) error); ok {
		r1 = rf(ctx, organizationId, contentType, body, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// CreateUserInviteWithResponse provides a mock function with given fields: ctx, organizationId, body, reqEditors
func (_m *ClientWithResponsesInterface) CreateUserInviteWithResponse(ctx context.Context, organizationId string, body astroiamcore.CreateUserInviteRequest, reqEditors ...astroiamcore.RequestEditorFn) (*astroiamcore.CreateUserInviteResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, organizationId, body)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astroiamcore.CreateUserInviteResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, astroiamcore.CreateUserInviteRequest, ...astroiamcore.RequestEditorFn) *astroiamcore.CreateUserInviteResponse); ok {
		r0 = rf(ctx, organizationId, body, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astroiamcore.CreateUserInviteResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, astroiamcore.CreateUserInviteRequest, ...astroiamcore.RequestEditorFn) error); ok {
		r1 = rf(ctx, organizationId, body, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// DeleteApiTokenWithResponse provides a mock function with given fields: ctx, organizationId, tokenId, reqEditors
func (_m *ClientWithResponsesInterface) DeleteApiTokenWithResponse(ctx context.Context, organizationId string, tokenId string, reqEditors ...astroiamcore.RequestEditorFn) (*astroiamcore.DeleteApiTokenResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, organizationId, tokenId)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astroiamcore.DeleteApiTokenResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, ...astroiamcore.RequestEditorFn) *astroiamcore.DeleteApiTokenResponse); ok {
		r0 = rf(ctx, organizationId, tokenId, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astroiamcore.DeleteApiTokenResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, ...astroiamcore.RequestEditorFn) error); ok {
		r1 = rf(ctx, organizationId, tokenId, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// DeleteTeamWithResponse provides a mock function with given fields: ctx, organizationId, teamId, reqEditors
func (_m *ClientWithResponsesInterface) DeleteTeamWithResponse(ctx context.Context, organizationId string, teamId string, reqEditors ...astroiamcore.RequestEditorFn) (*astroiamcore.DeleteTeamResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, organizationId, teamId)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astroiamcore.DeleteTeamResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, ...astroiamcore.RequestEditorFn) *astroiamcore.DeleteTeamResponse); ok {
		r0 = rf(ctx, organizationId, teamId, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astroiamcore.DeleteTeamResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, ...astroiamcore.RequestEditorFn) error); ok {
		r1 = rf(ctx, organizationId, teamId, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// DeleteUserInviteWithResponse provides a mock function with given fields: ctx, organizationId, inviteId, reqEditors
func (_m *ClientWithResponsesInterface) DeleteUserInviteWithResponse(ctx context.Context, organizationId string, inviteId string, reqEditors ...astroiamcore.RequestEditorFn) (*astroiamcore.DeleteUserInviteResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, organizationId, inviteId)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astroiamcore.DeleteUserInviteResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, ...astroiamcore.RequestEditorFn) *astroiamcore.DeleteUserInviteResponse); ok {
		r0 = rf(ctx, organizationId, inviteId, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astroiamcore.DeleteUserInviteResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, ...astroiamcore.RequestEditorFn) error); ok {
		r1 = rf(ctx, organizationId, inviteId, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetApiTokenWithResponse provides a mock function with given fields: ctx, organizationId, tokenId, reqEditors
func (_m *ClientWithResponsesInterface) GetApiTokenWithResponse(ctx context.Context, organizationId string, tokenId string, reqEditors ...astroiamcore.RequestEditorFn) (*astroiamcore.GetApiTokenResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, organizationId, tokenId)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astroiamcore.GetApiTokenResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, ...astroiamcore.RequestEditorFn) *astroiamcore.GetApiTokenResponse); ok {
		r0 = rf(ctx, organizationId, tokenId, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astroiamcore.GetApiTokenResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, ...astroiamcore.RequestEditorFn) error); ok {
		r1 = rf(ctx, organizationId, tokenId, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetTeamWithResponse provides a mock function with given fields: ctx, organizationId, teamId, reqEditors
func (_m *ClientWithResponsesInterface) GetTeamWithResponse(ctx context.Context, organizationId string, teamId string, reqEditors ...astroiamcore.RequestEditorFn) (*astroiamcore.GetTeamResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, organizationId, teamId)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astroiamcore.GetTeamResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, ...astroiamcore.RequestEditorFn) *astroiamcore.GetTeamResponse); ok {
		r0 = rf(ctx, organizationId, teamId, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astroiamcore.GetTeamResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, ...astroiamcore.RequestEditorFn) error); ok {
		r1 = rf(ctx, organizationId, teamId, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetUserWithResponse provides a mock function with given fields: ctx, organizationId, userId, reqEditors
func (_m *ClientWithResponsesInterface) GetUserWithResponse(ctx context.Context, organizationId string, userId string, reqEditors ...astroiamcore.RequestEditorFn) (*astroiamcore.GetUserResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, organizationId, userId)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astroiamcore.GetUserResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, ...astroiamcore.RequestEditorFn) *astroiamcore.GetUserResponse); ok {
		r0 = rf(ctx, organizationId, userId, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astroiamcore.GetUserResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, ...astroiamcore.RequestEditorFn) error); ok {
		r1 = rf(ctx, organizationId, userId, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ListApiTokensWithResponse provides a mock function with given fields: ctx, organizationId, params, reqEditors
func (_m *ClientWithResponsesInterface) ListApiTokensWithResponse(ctx context.Context, organizationId string, params *astroiamcore.ListApiTokensParams, reqEditors ...astroiamcore.RequestEditorFn) (*astroiamcore.ListApiTokensResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, organizationId, params)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astroiamcore.ListApiTokensResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, *astroiamcore.ListApiTokensParams, ...astroiamcore.RequestEditorFn) *astroiamcore.ListApiTokensResponse); ok {
		r0 = rf(ctx, organizationId, params, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astroiamcore.ListApiTokensResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, *astroiamcore.ListApiTokensParams, ...astroiamcore.RequestEditorFn) error); ok {
		r1 = rf(ctx, organizationId, params, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ListTeamMembersWithResponse provides a mock function with given fields: ctx, organizationId, teamId, params, reqEditors
func (_m *ClientWithResponsesInterface) ListTeamMembersWithResponse(ctx context.Context, organizationId string, teamId string, params *astroiamcore.ListTeamMembersParams, reqEditors ...astroiamcore.RequestEditorFn) (*astroiamcore.ListTeamMembersResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, organizationId, teamId, params)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astroiamcore.ListTeamMembersResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, *astroiamcore.ListTeamMembersParams, ...astroiamcore.RequestEditorFn) *astroiamcore.ListTeamMembersResponse); ok {
		r0 = rf(ctx, organizationId, teamId, params, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astroiamcore.ListTeamMembersResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, *astroiamcore.ListTeamMembersParams, ...astroiamcore.RequestEditorFn) error); ok {
		r1 = rf(ctx, organizationId, teamId, params, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ListTeamsWithResponse provides a mock function with given fields: ctx, organizationId, params, reqEditors
func (_m *ClientWithResponsesInterface) ListTeamsWithResponse(ctx context.Context, organizationId string, params *astroiamcore.ListTeamsParams, reqEditors ...astroiamcore.RequestEditorFn) (*astroiamcore.ListTeamsResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, organizationId, params)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astroiamcore.ListTeamsResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, *astroiamcore.ListTeamsParams, ...astroiamcore.RequestEditorFn) *astroiamcore.ListTeamsResponse); ok {
		r0 = rf(ctx, organizationId, params, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astroiamcore.ListTeamsResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, *astroiamcore.ListTeamsParams, ...astroiamcore.RequestEditorFn) error); ok {
		r1 = rf(ctx, organizationId, params, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ListUsersWithResponse provides a mock function with given fields: ctx, organizationId, params, reqEditors
func (_m *ClientWithResponsesInterface) ListUsersWithResponse(ctx context.Context, organizationId string, params *astroiamcore.ListUsersParams, reqEditors ...astroiamcore.RequestEditorFn) (*astroiamcore.ListUsersResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, organizationId, params)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astroiamcore.ListUsersResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, *astroiamcore.ListUsersParams, ...astroiamcore.RequestEditorFn) *astroiamcore.ListUsersResponse); ok {
		r0 = rf(ctx, organizationId, params, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astroiamcore.ListUsersResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, *astroiamcore.ListUsersParams, ...astroiamcore.RequestEditorFn) error); ok {
		r1 = rf(ctx, organizationId, params, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// RemoveTeamMemberWithResponse provides a mock function with given fields: ctx, organizationId, teamId, memberId, reqEditors
func (_m *ClientWithResponsesInterface) RemoveTeamMemberWithResponse(ctx context.Context, organizationId string, teamId string, memberId string, reqEditors ...astroiamcore.RequestEditorFn) (*astroiamcore.RemoveTeamMemberResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, organizationId, teamId, memberId)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astroiamcore.RemoveTeamMemberResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, string, ...astroiamcore.RequestEditorFn) *astroiamcore.RemoveTeamMemberResponse); ok {
		r0 = rf(ctx, organizationId, teamId, memberId, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astroiamcore.RemoveTeamMemberResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, string, ...astroiamcore.RequestEditorFn) error); ok {
		r1 = rf(ctx, organizationId, teamId, memberId, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// RotateApiTokenWithResponse provides a mock function with given fields: ctx, organizationId, tokenId, reqEditors
func (_m *ClientWithResponsesInterface) RotateApiTokenWithResponse(ctx context.Context, organizationId string, tokenId string, reqEditors ...astroiamcore.RequestEditorFn) (*astroiamcore.RotateApiTokenResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, organizationId, tokenId)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astroiamcore.RotateApiTokenResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, ...astroiamcore.RequestEditorFn) *astroiamcore.RotateApiTokenResponse); ok {
		r0 = rf(ctx, organizationId, tokenId, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astroiamcore.RotateApiTokenResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, ...astroiamcore.RequestEditorFn) error); ok {
		r1 = rf(ctx, organizationId, tokenId, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// UpdateApiTokenRolesWithBodyWithResponse provides a mock function with given fields: ctx, organizationId, tokenId, contentType, body, reqEditors
func (_m *ClientWithResponsesInterface) UpdateApiTokenRolesWithBodyWithResponse(ctx context.Context, organizationId string, tokenId string, contentType string, body io.Reader, reqEditors ...astroiamcore.RequestEditorFn) (*astroiamcore.UpdateApiTokenRolesResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, organizationId, tokenId, contentType, body)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astroiamcore.UpdateApiTokenRolesResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, string, io.Reader, ...astroiamcore.RequestEditorFn) *astroiamcore.UpdateApiTokenRolesResponse); ok {
		r0 = rf(ctx, organizationId, tokenId, contentType, body, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astroiamcore.UpdateApiTokenRolesResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, string, io.Reader, ...astroiamcore.RequestEditorFn) error); ok {
		r1 = rf(ctx, organizationId, tokenId, contentType, body, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// UpdateApiTokenRolesWithResponse provides a mock function with given fields: ctx, organizationId, tokenId, body, reqEditors
func (_m *ClientWithResponsesInterface) UpdateApiTokenRolesWithResponse(ctx context.Context, organizationId string, tokenId string, body astroiamcore.UpdateApiTokenRolesRequest, reqEditors ...astroiamcore.RequestEditorFn) (*astroiamcore.UpdateApiTokenRolesResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, organizationId, tokenId, body)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astroiamcore.UpdateApiTokenRolesResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, astroiamcore.UpdateApiTokenRolesRequest, ...astroiamcore.RequestEditorFn) *astroiamcore.UpdateApiTokenRolesResponse); ok {
		r0 = rf(ctx, organizationId, tokenId, body, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astroiamcore.UpdateApiTokenRolesResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, astroiamcore.UpdateApiTokenRolesRequest, ...astroiamcore.RequestEditorFn) error); ok {
		r1 = rf(ctx, organizationId, tokenId, body, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// UpdateApiTokenWithBodyWithResponse provides a mock function with given fields: ctx, organizationId, tokenId, contentType, body, reqEditors
func (_m *ClientWithResponsesInterface) UpdateApiTokenWithBodyWithResponse(ctx context.Context, organizationId string, tokenId string, contentType string, body io.Reader, reqEditors ...astroiamcore.RequestEditorFn) (*astroiamcore.UpdateApiTokenResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, organizationId, tokenId, contentType, body)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astroiamcore.UpdateApiTokenResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, string, io.Reader, ...astroiamcore.RequestEditorFn) *astroiamcore.UpdateApiTokenResponse); ok {
		r0 = rf(ctx, organizationId, tokenId, contentType, body, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astroiamcore.UpdateApiTokenResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, string, io.Reader, ...astroiamcore.RequestEditorFn) error); ok {
		r1 = rf(ctx, organizationId, tokenId, contentType, body, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// UpdateApiTokenWithResponse provides a mock function with given fields: ctx, organizationId, tokenId, body, reqEditors
func (_m *ClientWithResponsesInterface) UpdateApiTokenWithResponse(ctx context.Context, organizationId string, tokenId string, body astroiamcore.UpdateApiTokenRequest, reqEditors ...astroiamcore.RequestEditorFn) (*astroiamcore.UpdateApiTokenResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, organizationId, tokenId, body)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astroiamcore.UpdateApiTokenResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, astroiamcore.UpdateApiTokenRequest, ...astroiamcore.RequestEditorFn) *astroiamcore.UpdateApiTokenResponse); ok {
		r0 = rf(ctx, organizationId, tokenId, body, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astroiamcore.UpdateApiTokenResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, astroiamcore.UpdateApiTokenRequest, ...astroiamcore.RequestEditorFn) error); ok {
		r1 = rf(ctx, organizationId, tokenId, body, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// UpdateTeamRolesWithBodyWithResponse provides a mock function with given fields: ctx, organizationId, teamId, contentType, body, reqEditors
func (_m *ClientWithResponsesInterface) UpdateTeamRolesWithBodyWithResponse(ctx context.Context, organizationId string, teamId string, contentType string, body io.Reader, reqEditors ...astroiamcore.RequestEditorFn) (*astroiamcore.UpdateTeamRolesResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, organizationId, teamId, contentType, body)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astroiamcore.UpdateTeamRolesResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, string, io.Reader, ...astroiamcore.RequestEditorFn) *astroiamcore.UpdateTeamRolesResponse); ok {
		r0 = rf(ctx, organizationId, teamId, contentType, body, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astroiamcore.UpdateTeamRolesResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, string, io.Reader, ...astroiamcore.RequestEditorFn) error); ok {
		r1 = rf(ctx, organizationId, teamId, contentType, body, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// UpdateTeamRolesWithResponse provides a mock function with given fields: ctx, organizationId, teamId, body, reqEditors
func (_m *ClientWithResponsesInterface) UpdateTeamRolesWithResponse(ctx context.Context, organizationId string, teamId string, body astroiamcore.UpdateTeamRolesRequest, reqEditors ...astroiamcore.RequestEditorFn) (*astroiamcore.UpdateTeamRolesResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, organizationId, teamId, body)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astroiamcore.UpdateTeamRolesResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, astroiamcore.UpdateTeamRolesRequest, ...astroiamcore.RequestEditorFn) *astroiamcore.UpdateTeamRolesResponse); ok {
		r0 = rf(ctx, organizationId, teamId, body, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astroiamcore.UpdateTeamRolesResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, astroiamcore.UpdateTeamRolesRequest, ...astroiamcore.RequestEditorFn) error); ok {
		r1 = rf(ctx, organizationId, teamId, body, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// UpdateTeamWithBodyWithResponse provides a mock function with given fields: ctx, organizationId, teamId, contentType, body, reqEditors
func (_m *ClientWithResponsesInterface) UpdateTeamWithBodyWithResponse(ctx context.Context, organizationId string, teamId string, contentType string, body io.Reader, reqEditors ...astroiamcore.RequestEditorFn) (*astroiamcore.UpdateTeamResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, organizationId, teamId, contentType, body)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astroiamcore.UpdateTeamResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, string, io.Reader, ...astroiamcore.RequestEditorFn) *astroiamcore.UpdateTeamResponse); ok {
		r0 = rf(ctx, organizationId, teamId, contentType, body, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astroiamcore.UpdateTeamResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, string, io.Reader, ...astroiamcore.RequestEditorFn) error); ok {
		r1 = rf(ctx, organizationId, teamId, contentType, body, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// UpdateTeamWithResponse provides a mock function with given fields: ctx, organizationId, teamId, body, reqEditors
func (_m *ClientWithResponsesInterface) UpdateTeamWithResponse(ctx context.Context, organizationId string, teamId string, body astroiamcore.UpdateTeamRequest, reqEditors ...astroiamcore.RequestEditorFn) (*astroiamcore.UpdateTeamResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, organizationId, teamId, body)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astroiamcore.UpdateTeamResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, astroiamcore.UpdateTeamRequest, ...astroiamcore.RequestEditorFn) *astroiamcore.UpdateTeamResponse); ok {
		r0 = rf(ctx, organizationId, teamId, body, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astroiamcore.UpdateTeamResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, astroiamcore.UpdateTeamRequest, ...astroiamcore.RequestEditorFn) error); ok {
		r1 = rf(ctx, organizationId, teamId, body, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// UpdateUserRolesWithBodyWithResponse provides a mock function with given fields: ctx, organizationId, userId, contentType, body, reqEditors
func (_m *ClientWithResponsesInterface) UpdateUserRolesWithBodyWithResponse(ctx context.Context, organizationId string, userId string, contentType string, body io.Reader, reqEditors ...astroiamcore.RequestEditorFn) (*astroiamcore.UpdateUserRolesResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, organizationId, userId, contentType, body)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astroiamcore.UpdateUserRolesResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, string, io.Reader, ...astroiamcore.RequestEditorFn) *astroiamcore.UpdateUserRolesResponse); ok {
		r0 = rf(ctx, organizationId, userId, contentType, body, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astroiamcore.UpdateUserRolesResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, string, io.Reader, ...astroiamcore.RequestEditorFn) error); ok {
		r1 = rf(ctx, organizationId, userId, contentType, body, reqEditors...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// UpdateUserRolesWithResponse provides a mock function with given fields: ctx, organizationId, userId, body, reqEditors
func (_m *ClientWithResponsesInterface) UpdateUserRolesWithResponse(ctx context.Context, organizationId string, userId string, body astroiamcore.UpdateUserRolesRequest, reqEditors ...astroiamcore.RequestEditorFn) (*astroiamcore.UpdateUserRolesResponse, error) {
	_va := make([]interface{}, len(reqEditors))
	for _i := range reqEditors {
		_va[_i] = reqEditors[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, organizationId, userId, body)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *astroiamcore.UpdateUserRolesResponse
	if rf, ok := ret.Get(0).(func(context.Context, string, string, astroiamcore.UpdateUserRolesRequest, ...astroiamcore.RequestEditorFn) *astroiamcore.UpdateUserRolesResponse); ok {
		r0 = rf(ctx, organizationId, userId, body, reqEditors...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*astroiamcore.UpdateUserRolesResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, astroiamcore.UpdateUserRolesRequest, ...astroiamcore.RequestEditorFn) error); ok {
		r1 = rf(ctx, organizationId, userId, body, reqEditors...)
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
