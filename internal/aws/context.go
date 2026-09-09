package aws

import (
	"context"
	"fmt"
	"strings"

	sdkaws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// ResolvedContext is the shared SDK configuration and caller scope used to
// construct service-specific adapters.
type ResolvedContext struct {
	Config    sdkaws.Config
	Partition string
	AccountID string
	Region    string
	CallerARN string
}

type callerIdentityAPI interface {
	GetCallerIdentity(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

type configLoader func(context.Context, ...func(*config.LoadOptions) error) (sdkaws.Config, error)
type stsFactory func(sdkaws.Config) callerIdentityAPI

// ResolveContext loads AWS configuration and resolves the caller exactly once.
func ResolveContext(ctx context.Context, opts ClientOptions) (*ResolvedContext, error) {
	return resolveContext(ctx, opts, config.LoadDefaultConfig, func(cfg sdkaws.Config) callerIdentityAPI {
		return sts.NewFromConfig(cfg)
	})
}

func resolveContext(ctx context.Context, opts ClientOptions, load configLoader, newSTS stsFactory) (*ResolvedContext, error) {
	var cfgOpts []func(*config.LoadOptions) error
	if opts.Region != "" {
		cfgOpts = append(cfgOpts, config.WithRegion(opts.Region))
	}
	if opts.ProfileSet {
		cfgOpts = append(cfgOpts, config.WithSharedConfigProfile(opts.Profile))
	}

	cfg, err := load(ctx, cfgOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}
	if strings.TrimSpace(cfg.Region) == "" {
		return nil, fmt.Errorf("AWS region is not configured; pass --region, set config region, or configure AWS_REGION/a shared AWS profile")
	}

	identity, err := newSTS(cfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to get AWS account ID: %w", err)
	}
	if identity == nil {
		return nil, fmt.Errorf("failed to get AWS account ID: STS returned an empty identity")
	}
	callerARN := sdkaws.ToString(identity.Arn)
	partition, err := partitionFromCallerARN(callerARN)
	if err != nil {
		return nil, err
	}

	return &ResolvedContext{
		Config:    cfg,
		Partition: partition,
		AccountID: sdkaws.ToString(identity.Account),
		Region:    cfg.Region,
		CallerARN: callerARN,
	}, nil
}

func partitionFromCallerARN(callerARN string) (string, error) {
	parsed, err := arn.Parse(callerARN)
	if err != nil || parsed.Partition == "" {
		return "", fmt.Errorf("failed to derive AWS partition from caller ARN %q", callerARN)
	}
	return parsed.Partition, nil
}
