package main

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// awsIAMArgs is the plain-Go carrier for every cloud.aws(...).iam() method:
// extracted on-loop by awsIAMExtract, consumed off-loop by the awsIAMXxx work
// funcs.
type awsIAMArgs struct {
	userName, roleName, policyArn string
}

// newIAMClient builds an iam.Client for cfg. Like EC2, IAM needs no
// UsePathStyle option — its query/XML wire protocol addresses everything by
// action + BaseEndpoint, not by URL host/path.
func newIAMClient(ctx context.Context, cfg awsConfig) (*iam.Client, error) {
	base, err := cfg.load(ctx)
	if err != nil {
		return nil, err
	}
	return iam.NewFromConfig(base, func(o *iam.Options) {
		if ep := awsBaseEndpoint(); ep != nil {
			o.BaseEndpoint = ep
		}
	}), nil
}

func awsIAMListUsers(ctx context.Context, cfg awsConfig, a awsIAMArgs) (any, error) {
	c, err := newIAMClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	out, err := c.ListUsers(ctx, &iam.ListUsersInput{})
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

func awsIAMGetUser(ctx context.Context, cfg awsConfig, a awsIAMArgs) (any, error) {
	c, err := newIAMClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	in := &iam.GetUserInput{}
	if a.userName != "" {
		in.UserName = aws.String(a.userName)
	}
	out, err := c.GetUser(ctx, in)
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

func awsIAMListRoles(ctx context.Context, cfg awsConfig, a awsIAMArgs) (any, error) {
	c, err := newIAMClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	out, err := c.ListRoles(ctx, &iam.ListRolesInput{})
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

func awsIAMGetRole(ctx context.Context, cfg awsConfig, a awsIAMArgs) (any, error) {
	c, err := newIAMClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	out, err := c.GetRole(ctx, &iam.GetRoleInput{RoleName: aws.String(a.roleName)})
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

func awsIAMListPolicies(ctx context.Context, cfg awsConfig, a awsIAMArgs) (any, error) {
	c, err := newIAMClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	out, err := c.ListPolicies(ctx, &iam.ListPoliciesInput{})
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

func awsIAMCreateUser(ctx context.Context, cfg awsConfig, a awsIAMArgs) (any, error) {
	c, err := newIAMClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	out, err := c.CreateUser(ctx, &iam.CreateUserInput{UserName: aws.String(a.userName)})
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

func awsIAMDeleteUser(ctx context.Context, cfg awsConfig, a awsIAMArgs) (any, error) {
	c, err := newIAMClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if _, err := c.DeleteUser(ctx, &iam.DeleteUserInput{UserName: aws.String(a.userName)}); err != nil {
		return nil, mapAWSError(err)
	}
	return map[string]any{}, nil
}

func awsIAMAttachUserPolicy(ctx context.Context, cfg awsConfig, a awsIAMArgs) (any, error) {
	c, err := newIAMClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if _, err := c.AttachUserPolicy(ctx, &iam.AttachUserPolicyInput{
		UserName:  aws.String(a.userName),
		PolicyArn: aws.String(a.policyArn),
	}); err != nil {
		return nil, mapAWSError(err)
	}
	return map[string]any{}, nil
}

// awsIAMExtract reads the single options object on the event loop.
func awsIAMExtract(call goja.FunctionCall) (awsIAMArgs, error) {
	var a awsIAMArgs
	obj, ok := call.Argument(0).(*goja.Object)
	if !ok {
		return a, nil
	}
	o, ok := obj.Export().(map[string]any)
	if !ok {
		return a, nil
	}
	a.userName = optString(o, "userName", "")
	a.roleName = optString(o, "roleName", "")
	a.policyArn = optString(o, "policyArn", "")
	return a, nil
}

// awsIAM builds the object returned by cloud.aws(...).iam(): one
// PromisifyAsync binding per method, all sharing awsIAMExtract and cfg.
//
// This map is built at script-run time (inside the iam() accessor call in
// cloud_aws.go), past the engine's registration-time AsyncBinding unwrap — so
// each binding's `.Func` must be unwrapped explicitly here (same pattern as
// awsS3/awsEC2).
func awsIAM(vm *goja.Runtime, loop *eventloop.EventLoop, cfg awsConfig) map[string]any {
	bind := func(fn func(context.Context, awsConfig, awsIAMArgs) (any, error)) func(goja.FunctionCall) goja.Value {
		return scriptengine.PromisifyAsync(vm, loop, awsIAMExtract,
			func(ctx context.Context, a awsIAMArgs) (any, error) { return fn(ctx, cfg, a) }).Func
	}
	return map[string]any{
		"listUsers":        bind(awsIAMListUsers),
		"getUser":          bind(awsIAMGetUser),
		"listRoles":        bind(awsIAMListRoles),
		"getRole":          bind(awsIAMGetRole),
		"listPolicies":     bind(awsIAMListPolicies),
		"createUser":       bind(awsIAMCreateUser),
		"deleteUser":       bind(awsIAMDeleteUser),
		"attachUserPolicy": bind(awsIAMAttachUserPolicy),
	}
}
