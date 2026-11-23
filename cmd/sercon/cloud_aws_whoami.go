package main

import (
	"context"
	"errors"

	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// awsLoginHint is appended to every UNAUTHENTICATED message so a script that
// surfaces the error tells the user how to fix it.
const awsLoginHint = "not logged in to AWS; run `aws sso login` (or `aws configure`)"

// awsAuthErrorCodes are STS API error codes that mean "these credentials are
// not (or no longer) valid", as opposed to "valid but not permitted".
var awsAuthErrorCodes = map[string]bool{
	"InvalidClientTokenId":        true,
	"ExpiredToken":                true,
	"ExpiredTokenException":       true,
	"UnrecognizedClientException": true,
	"SignatureDoesNotMatch":       true,
}

// classifyAWSAuthError maps err to an awsError. Credential-retrieval failures
// (raised while signing: no creds, expired SSO session, …) and STS codes in
// awsAuthErrorCodes become code "UNAUTHENTICATED" with the login hint; every
// other error keeps the plain mapAWSError mapping (so a network failure is
// never reported as "not logged in").
func classifyAWSAuthError(err error) error {
	if err == nil {
		return nil
	}
	base := mapAWSError(err)
	ae, _ := base.(awsError)
	var signErr *v4.SigningError
	cause := ae.code
	switch {
	case errors.As(err, &signErr):
		cause = "credentials"
	case awsAuthErrorCodes[ae.code]:
	default:
		return base
	}
	msg := ae.message
	if ae.code != "" {
		msg = ae.code + ": " + msg
	}
	return awsError{
		code:    "UNAUTHENTICATED",
		status:  ae.status,
		message: msg + " — " + awsLoginHint,
		details: map[string]any{"cause": cause},
	}
}

// awsWhoami returns the identity behind cfg's credentials as
// { account, arn, userId }, or an UNAUTHENTICATED awsError when there is no
// usable login. A config/profile load failure counts as not logged in.
func awsWhoami(ctx context.Context, cfg awsConfig) (map[string]any, error) {
	c, err := newSTSClient(ctx, cfg)
	if err != nil {
		msg := err.Error()
		if ae, ok := err.(awsError); ok {
			msg = ae.message
		}
		return nil, awsError{
			code:    "UNAUTHENTICATED",
			message: msg + " — " + awsLoginHint,
			details: map[string]any{"cause": "config"},
		}
	}
	out, err := c.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, classifyAWSAuthError(err)
	}
	deref := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	return map[string]any{
		"account": deref(out.Account),
		"arn":     deref(out.Arn),
		"userId":  deref(out.UserId),
	}, nil
}
