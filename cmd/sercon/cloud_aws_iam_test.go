package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAWSIAM_ListUsers(t *testing.T) {
	var gotAction string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotAction = r.Form.Get("Action")
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<ListUsersResponse xmlns="https://iam.amazonaws.com/doc/2010-05-08/">
  <ListUsersResult>
    <Users>
      <member>
        <UserName>alice</UserName>
        <UserId>AIDAEXAMPLE1</UserId>
        <Arn>arn:aws:iam::123456789012:user/alice</Arn>
        <Path>/</Path>
        <CreateDate>2024-01-01T00:00:00Z</CreateDate>
      </member>
    </Users>
    <IsTruncated>false</IsTruncated>
  </ListUsersResult>
  <ResponseMetadata>
    <RequestId>req-1</RequestId>
  </ResponseMetadata>
</ListUsersResponse>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsIAMListUsers(context.Background(), awsConfig{}, awsIAMArgs{})
	if err != nil {
		t.Fatalf("listUsers: %v", err)
	}
	if gotAction != "ListUsers" {
		t.Fatalf("expected Action=ListUsers, got %q", gotAction)
	}
	m := out.(map[string]any)
	users, ok := m["Users"].([]any)
	if !ok || len(users) != 1 {
		t.Fatalf("expected 1 user, got %#v", m["Users"])
	}
	u := users[0].(map[string]any)
	if u["UserName"] != "alice" {
		t.Fatalf("expected UserName alice, got %#v", u["UserName"])
	}
}

func TestAWSIAM_GetUser(t *testing.T) {
	var gotAction, gotUserName string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotAction = r.Form.Get("Action")
		gotUserName = r.Form.Get("UserName")
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<GetUserResponse xmlns="https://iam.amazonaws.com/doc/2010-05-08/">
  <GetUserResult>
    <User>
      <UserName>bob</UserName>
      <UserId>AIDAEXAMPLE2</UserId>
      <Arn>arn:aws:iam::123456789012:user/bob</Arn>
      <Path>/</Path>
      <CreateDate>2024-01-01T00:00:00Z</CreateDate>
    </User>
  </GetUserResult>
  <ResponseMetadata>
    <RequestId>req-2</RequestId>
  </ResponseMetadata>
</GetUserResponse>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsIAMGetUser(context.Background(), awsConfig{}, awsIAMArgs{userName: "bob"})
	if err != nil {
		t.Fatalf("getUser: %v", err)
	}
	if gotAction != "GetUser" {
		t.Fatalf("expected Action=GetUser, got %q", gotAction)
	}
	if gotUserName != "bob" {
		t.Fatalf("expected UserName bob, got %q", gotUserName)
	}
	m := out.(map[string]any)
	u, ok := m["User"].(map[string]any)
	if !ok {
		t.Fatalf("expected User object, got %#v", m["User"])
	}
	if u["UserName"] != "bob" {
		t.Fatalf("expected UserName bob, got %#v", u["UserName"])
	}
}

func TestAWSIAM_ListRoles(t *testing.T) {
	var gotAction string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotAction = r.Form.Get("Action")
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<ListRolesResponse xmlns="https://iam.amazonaws.com/doc/2010-05-08/">
  <ListRolesResult>
    <Roles>
      <member>
        <RoleName>my-role</RoleName>
        <RoleId>AROAEXAMPLE1</RoleId>
        <Arn>arn:aws:iam::123456789012:role/my-role</Arn>
        <Path>/</Path>
        <CreateDate>2024-01-01T00:00:00Z</CreateDate>
      </member>
    </Roles>
    <IsTruncated>false</IsTruncated>
  </ListRolesResult>
  <ResponseMetadata>
    <RequestId>req-3</RequestId>
  </ResponseMetadata>
</ListRolesResponse>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsIAMListRoles(context.Background(), awsConfig{}, awsIAMArgs{})
	if err != nil {
		t.Fatalf("listRoles: %v", err)
	}
	if gotAction != "ListRoles" {
		t.Fatalf("expected Action=ListRoles, got %q", gotAction)
	}
	m := out.(map[string]any)
	roles, ok := m["Roles"].([]any)
	if !ok || len(roles) != 1 {
		t.Fatalf("expected 1 role, got %#v", m["Roles"])
	}
	r := roles[0].(map[string]any)
	if r["RoleName"] != "my-role" {
		t.Fatalf("expected RoleName my-role, got %#v", r["RoleName"])
	}
}

func TestAWSIAM_GetRole(t *testing.T) {
	var gotAction, gotRoleName string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotAction = r.Form.Get("Action")
		gotRoleName = r.Form.Get("RoleName")
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<GetRoleResponse xmlns="https://iam.amazonaws.com/doc/2010-05-08/">
  <GetRoleResult>
    <Role>
      <RoleName>my-role</RoleName>
      <RoleId>AROAEXAMPLE1</RoleId>
      <Arn>arn:aws:iam::123456789012:role/my-role</Arn>
      <Path>/</Path>
      <CreateDate>2024-01-01T00:00:00Z</CreateDate>
    </Role>
  </GetRoleResult>
  <ResponseMetadata>
    <RequestId>req-4</RequestId>
  </ResponseMetadata>
</GetRoleResponse>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsIAMGetRole(context.Background(), awsConfig{}, awsIAMArgs{roleName: "my-role"})
	if err != nil {
		t.Fatalf("getRole: %v", err)
	}
	if gotAction != "GetRole" {
		t.Fatalf("expected Action=GetRole, got %q", gotAction)
	}
	if gotRoleName != "my-role" {
		t.Fatalf("expected RoleName my-role, got %q", gotRoleName)
	}
	m := out.(map[string]any)
	role, ok := m["Role"].(map[string]any)
	if !ok {
		t.Fatalf("expected Role object, got %#v", m["Role"])
	}
	if role["RoleName"] != "my-role" {
		t.Fatalf("expected RoleName my-role, got %#v", role["RoleName"])
	}
}

func TestAWSIAM_ListPolicies(t *testing.T) {
	var gotAction string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotAction = r.Form.Get("Action")
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<ListPoliciesResponse xmlns="https://iam.amazonaws.com/doc/2010-05-08/">
  <ListPoliciesResult>
    <Policies>
      <member>
        <PolicyName>my-policy</PolicyName>
        <PolicyId>ANPAEXAMPLE1</PolicyId>
        <Arn>arn:aws:iam::123456789012:policy/my-policy</Arn>
        <Path>/</Path>
        <CreateDate>2024-01-01T00:00:00Z</CreateDate>
        <UpdateDate>2024-01-01T00:00:00Z</UpdateDate>
      </member>
    </Policies>
    <IsTruncated>false</IsTruncated>
  </ListPoliciesResult>
  <ResponseMetadata>
    <RequestId>req-5</RequestId>
  </ResponseMetadata>
</ListPoliciesResponse>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsIAMListPolicies(context.Background(), awsConfig{}, awsIAMArgs{})
	if err != nil {
		t.Fatalf("listPolicies: %v", err)
	}
	if gotAction != "ListPolicies" {
		t.Fatalf("expected Action=ListPolicies, got %q", gotAction)
	}
	m := out.(map[string]any)
	policies, ok := m["Policies"].([]any)
	if !ok || len(policies) != 1 {
		t.Fatalf("expected 1 policy, got %#v", m["Policies"])
	}
	p := policies[0].(map[string]any)
	if p["PolicyName"] != "my-policy" {
		t.Fatalf("expected PolicyName my-policy, got %#v", p["PolicyName"])
	}
}

func TestAWSIAM_CreateUser(t *testing.T) {
	var gotAction, gotUserName string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotAction = r.Form.Get("Action")
		gotUserName = r.Form.Get("UserName")
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<CreateUserResponse xmlns="https://iam.amazonaws.com/doc/2010-05-08/">
  <CreateUserResult>
    <User>
      <UserName>new-user</UserName>
      <UserId>AIDAEXAMPLE3</UserId>
      <Arn>arn:aws:iam::123456789012:user/new-user</Arn>
      <Path>/</Path>
      <CreateDate>2024-01-01T00:00:00Z</CreateDate>
    </User>
  </CreateUserResult>
  <ResponseMetadata>
    <RequestId>req-6</RequestId>
  </ResponseMetadata>
</CreateUserResponse>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsIAMCreateUser(context.Background(), awsConfig{}, awsIAMArgs{userName: "new-user"})
	if err != nil {
		t.Fatalf("createUser: %v", err)
	}
	if gotAction != "CreateUser" {
		t.Fatalf("expected Action=CreateUser, got %q", gotAction)
	}
	if gotUserName != "new-user" {
		t.Fatalf("expected UserName new-user, got %q", gotUserName)
	}
	m := out.(map[string]any)
	u, ok := m["User"].(map[string]any)
	if !ok {
		t.Fatalf("expected User object, got %#v", m["User"])
	}
	if u["UserName"] != "new-user" {
		t.Fatalf("expected UserName new-user, got %#v", u["UserName"])
	}
}

func TestAWSIAM_DeleteUser(t *testing.T) {
	var gotAction, gotUserName string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotAction = r.Form.Get("Action")
		gotUserName = r.Form.Get("UserName")
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<DeleteUserResponse xmlns="https://iam.amazonaws.com/doc/2010-05-08/">
  <ResponseMetadata>
    <RequestId>req-7</RequestId>
  </ResponseMetadata>
</DeleteUserResponse>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsIAMDeleteUser(context.Background(), awsConfig{}, awsIAMArgs{userName: "old-user"})
	if err != nil {
		t.Fatalf("deleteUser: %v", err)
	}
	if gotAction != "DeleteUser" {
		t.Fatalf("expected Action=DeleteUser, got %q", gotAction)
	}
	if gotUserName != "old-user" {
		t.Fatalf("expected UserName old-user, got %q", gotUserName)
	}
	m, ok := out.(map[string]any)
	if !ok || len(m) != 0 {
		t.Fatalf("expected empty object, got %#v", out)
	}
}

func TestAWSIAM_AttachUserPolicy(t *testing.T) {
	var gotAction, gotUserName, gotPolicyArn string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotAction = r.Form.Get("Action")
		gotUserName = r.Form.Get("UserName")
		gotPolicyArn = r.Form.Get("PolicyArn")
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<AttachUserPolicyResponse xmlns="https://iam.amazonaws.com/doc/2010-05-08/">
  <ResponseMetadata>
    <RequestId>req-8</RequestId>
  </ResponseMetadata>
</AttachUserPolicyResponse>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsIAMAttachUserPolicy(context.Background(), awsConfig{}, awsIAMArgs{
		userName: "alice", policyArn: "arn:aws:iam::aws:policy/ReadOnlyAccess",
	})
	if err != nil {
		t.Fatalf("attachUserPolicy: %v", err)
	}
	if gotAction != "AttachUserPolicy" {
		t.Fatalf("expected Action=AttachUserPolicy, got %q", gotAction)
	}
	if gotUserName != "alice" {
		t.Fatalf("expected UserName alice, got %q", gotUserName)
	}
	if gotPolicyArn != "arn:aws:iam::aws:policy/ReadOnlyAccess" {
		t.Fatalf("expected PolicyArn arn:aws:iam::aws:policy/ReadOnlyAccess, got %q", gotPolicyArn)
	}
	m, ok := out.(map[string]any)
	if !ok || len(m) != 0 {
		t.Fatalf("expected empty object, got %#v", out)
	}
}

// TestAWSIAM_ErrorPathThrows mirrors TestAWSEC2_ErrorPathThrows: proves an IAM
// query-protocol error response is mapped end to end (SDK response -> smithy
// APIError -> mapAWSError) into a structured awsError.
func TestAWSIAM_ErrorPathThrows(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<ErrorResponse xmlns="https://iam.amazonaws.com/doc/2010-05-08/">
  <Error>
    <Type>Sender</Type>
    <Code>NoSuchEntity</Code>
    <Message>The user with name ghost cannot be found.</Message>
  </Error>
  <RequestId>req-err</RequestId>
</ErrorResponse>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsIAMGetUser(context.Background(), awsConfig{}, awsIAMArgs{userName: "ghost"})
	if err == nil {
		t.Fatalf("expected error, got nil (out=%#v)", out)
	}
	ae, ok := err.(awsError)
	if !ok {
		t.Fatalf("expected awsError, got %T: %v", err, err)
	}
	if ae.status != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", ae.status)
	}
	if ae.code != "NoSuchEntity" {
		t.Fatalf("expected code NoSuchEntity, got %q", ae.code)
	}
}

func TestAWSIAM_ListUsers_ViaJS(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<ListUsersResponse xmlns="https://iam.amazonaws.com/doc/2010-05-08/">
  <ListUsersResult>
    <Users>
      <member>
        <UserName>alice</UserName>
        <UserId>AIDAEXAMPLE1</UserId>
        <Arn>arn:aws:iam::123456789012:user/alice</Arn>
        <Path>/</Path>
        <CreateDate>2024-01-01T00:00:00Z</CreateDate>
      </member>
    </Users>
    <IsTruncated>false</IsTruncated>
  </ListUsersResult>
  <ResponseMetadata>
    <RequestId>req-js-1</RequestId>
  </ResponseMetadata>
</ListUsersResponse>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	got := runCloudAWSScript(t, `
		const __result = await cloud.aws({ region: "eu-north-1" }).iam().listUsers();
	`)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected object result, got %#v", got)
	}
	users, ok := m["Users"].([]any)
	if !ok || len(users) != 1 {
		t.Fatalf("expected 1 user, got %#v", m["Users"])
	}
}
