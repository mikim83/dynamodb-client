package awsconfig

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
)

// Load builds the SDK config for profile (empty: the SDK default chain) and
// region (empty: the profile's region). It resolves the credentials right
// away, so an MFA code is asked on the terminal before the UI starts and
// expired SSO sessions fail here with a clear message.
func Load(ctx context.Context, profile, region string) (aws.Config, error) {
	opts := []func(*config.LoadOptions) error{
		config.WithAssumeRoleCredentialOptions(func(o *stscreds.AssumeRoleOptions) {
			o.TokenProvider = stscreds.StdinTokenProvider
		}),
	}
	if profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(profile))
	}
	if region != "" {
		opts = append(opts, config.WithRegion(region))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return aws.Config{}, err
	}
	if cfg.Region == "" {
		return aws.Config{}, errors.New("no region set: pass --region or set region in the profile")
	}
	if _, err := cfg.Credentials.Retrieve(ctx); err != nil {
		return aws.Config{}, explain(err, profile)
	}
	return cfg, nil
}

func explain(err error, profile string) error {
	msg := err.Error()
	if strings.Contains(msg, "sso") || strings.Contains(msg, "SSO") {
		login := "aws sso login"
		if profile != "" {
			login += " --profile " + profile
		}
		return fmt.Errorf("%w\nhint: the SSO session may have expired, run `%s`", err, login)
	}
	return err
}
