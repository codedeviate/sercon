package main

import (
	"context"
	"errors"

	"net"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/sts"
	smithyhttp "github.com/aws/smithy-go/transport/http"
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

// classifyAWSAuthError maps an STS call error to an awsError: STS codes in
// awsAuthErrorCodes become code "UNAUTHENTICATED" with the login hint;
// everything else keeps the plain mapAWSError mapping (so a network failure
// is never reported as "not logged in").
func classifyAWSAuthError(err error) error {
	if err == nil {
		return nil
	}
	base := mapAWSError(err)
	ae, _ := base.(awsError)
	if !awsAuthErrorCodes[ae.code] {
		return base
	}
	return awsUnauthenticated(ae, ae.code)
}

// classifyAWSCredsError maps a credential-retrieval failure. Retrieval
// failing means there is no usable login (no credentials, an expired SSO
// session or refresh token, …) — UNAUTHENTICATED — unless it failed because
// the network is down (a send/DNS error), which keeps the transport mapping.
// The default chain's last resort, the EC2 IMDS role lookup, fails with a
// network error on any machine that is not an EC2 instance: that still means
// "no credentials", not "offline".
func classifyAWSCredsError(err error) error {
	base := mapAWSError(err)
	ae, _ := base.(awsError)
	var sendErr *smithyhttp.RequestSendError
	var netErr net.Error
	imds := strings.Contains(err.Error(), "no EC2 IMDS role found")
	if !imds && (errors.As(err, &sendErr) || errors.As(err, &netErr)) {
		return base
	}
	return awsUnauthenticated(ae, "credentials")
}

func awsUnauthenticated(ae awsError, cause string) error {
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
// usable login. Credentials are retrieved before the STS call so a retrieval
// failure is classified on its own (the STS client would otherwise wrap it
// as "get identity: …" and retry it). A config/profile load failure counts as
// not logged in.
func awsWhoami(ctx context.Context, cfg awsConfig) (map[string]any, error) {
	base, err := cfg.load(ctx)
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
	if base.Credentials == nil {
		return nil, awsUnauthenticated(awsError{message: "no credential provider configured"}, "credentials")
	}
	if _, err := base.Credentials.Retrieve(ctx); err != nil {
		return nil, classifyAWSCredsError(err)
	}
	c := sts.NewFromConfig(base, func(o *sts.Options) {
		if ep := awsBaseEndpoint(); ep != nil {
			o.BaseEndpoint = ep
		}
	})
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
