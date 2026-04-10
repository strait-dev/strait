package registry

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// ECRConfig holds configuration for an AWS ECR-backed container registry.
type ECRConfig struct {
	// Region is the AWS region hosting the ECR registry (e.g. "us-east-1").
	Region string
	// RegistryID is the AWS account ID that owns the ECR registry.
	// If empty, the caller's account ID is used.
	RegistryID string
	// RoleARN is an optional IAM role ARN to assume when calling ECR APIs.
	// Useful for cross-account access or least-privilege workload identity.
	RoleARN string
	// RepositoryPrefix is prepended to all repository names.
	// Defaults to "strait-jobs" when empty.
	RepositoryPrefix string
}

// ECRRegistry implements ContainerRegistry using AWS Elastic Container Registry.
type ECRRegistry struct {
	client           *ecr.Client
	registryID       string
	repositoryPrefix string
}

// NewECRRegistry creates a new ECRRegistry from cfg.
func NewECRRegistry(ctx context.Context, cfg ECRConfig) (*ECRRegistry, error) {
	if cfg.Region == "" {
		return nil, errors.New("registry: ECR region is required")
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.Region))
	if err != nil {
		return nil, fmt.Errorf("registry: load AWS config: %w", err)
	}

	if cfg.RoleARN != "" {
		stsSvc := sts.NewFromConfig(awsCfg)
		creds := stscreds.NewAssumeRoleProvider(stsSvc, cfg.RoleARN)
		awsCfg.Credentials = aws.NewCredentialsCache(creds)
	}

	prefix := cfg.RepositoryPrefix
	if prefix == "" {
		prefix = "strait-jobs"
	}

	return &ECRRegistry{
		client:           ecr.NewFromConfig(awsCfg),
		registryID:       cfg.RegistryID,
		repositoryPrefix: prefix,
	}, nil
}

// EnsureRepository creates the ECR repository if it does not exist and returns its URI.
func (e *ECRRegistry) EnsureRepository(ctx context.Context, name string) (string, error) {
	repoName := e.repositoryPrefix + "/" + name

	// Try to describe the repository first (cheaper than always creating).
	input := &ecr.DescribeRepositoriesInput{
		RepositoryNames: []string{repoName},
	}
	if e.registryID != "" {
		input.RegistryId = aws.String(e.registryID)
	}

	out, err := e.client.DescribeRepositories(ctx, input)
	if err != nil {
		var notFound *ecrtypes.RepositoryNotFoundException
		if !errors.As(err, &notFound) {
			return "", fmt.Errorf("registry: describe repository: %w", err)
		}
		// Repository does not exist — create it.
		return e.createRepository(ctx, repoName)
	}

	if len(out.Repositories) == 0 {
		return e.createRepository(ctx, repoName)
	}
	return aws.ToString(out.Repositories[0].RepositoryUri), nil
}

func (e *ECRRegistry) createRepository(ctx context.Context, name string) (string, error) {
	input := &ecr.CreateRepositoryInput{
		RepositoryName:     aws.String(name),
		ImageTagMutability: ecrtypes.ImageTagMutabilityImmutable,
		ImageScanningConfiguration: &ecrtypes.ImageScanningConfiguration{
			ScanOnPush: true,
		},
	}
	if e.registryID != "" {
		input.RegistryId = aws.String(e.registryID)
	}

	out, err := e.client.CreateRepository(ctx, input)
	if err != nil {
		var alreadyExists *ecrtypes.RepositoryAlreadyExistsException
		if errors.As(err, &alreadyExists) {
			// Race condition: another goroutine created it first. Describe to get URI.
			return e.EnsureRepository(ctx, strings.TrimPrefix(name, e.repositoryPrefix+"/"))
		}
		return "", fmt.Errorf("registry: create repository: %w", err)
	}
	return aws.ToString(out.Repository.RepositoryUri), nil
}

// GetAuthToken returns a base64-encoded ECR authorization token.
func (e *ECRRegistry) GetAuthToken(ctx context.Context) (string, time.Time, error) {
	input := &ecr.GetAuthorizationTokenInput{}

	out, err := e.client.GetAuthorizationToken(ctx, input)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("registry: get auth token: %w", err)
	}
	if len(out.AuthorizationData) == 0 {
		return "", time.Time{}, fmt.Errorf("registry: get auth token: no data returned")
	}

	data := out.AuthorizationData[0]
	token := aws.ToString(data.AuthorizationToken)
	expiresAt := aws.ToTime(data.ExpiresAt)

	// ECR returns a base64-encoded "AWS:token" string; decode to get the raw token,
	// then strip the "AWS:" prefix — BuildKit expects just the password.
	decoded, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("registry: decode auth token: %w", err)
	}
	// decoded is "AWS:{password}" — return the full decoded value for Docker auth.
	return string(decoded), expiresAt, nil
}

// GetImageDigest returns the digest of the image at repositoryURI:tag.
func (e *ECRRegistry) GetImageDigest(ctx context.Context, repositoryURI, tag string) (string, error) {
	// Extract repo name from URI: "{account}.dkr.ecr.{region}.amazonaws.com/{name}"
	repoName := repositoryURIToName(repositoryURI)

	input := &ecr.DescribeImagesInput{
		RepositoryName: aws.String(repoName),
		ImageIds: []ecrtypes.ImageIdentifier{
			{ImageTag: aws.String(tag)},
		},
	}
	if e.registryID != "" {
		input.RegistryId = aws.String(e.registryID)
	}

	out, err := e.client.DescribeImages(ctx, input)
	if err != nil {
		var notFound *ecrtypes.ImageNotFoundException
		if errors.As(err, &notFound) {
			return "", ErrImageNotFound
		}
		return "", fmt.Errorf("registry: describe image: %w", err)
	}
	if len(out.ImageDetails) == 0 {
		return "", ErrImageNotFound
	}
	return aws.ToString(out.ImageDetails[0].ImageDigest), nil
}

// repositoryURIToName extracts the repository name from a full ECR URI.
// "123456.dkr.ecr.us-east-1.amazonaws.com/strait-jobs/proj/job" → "strait-jobs/proj/job".
func repositoryURIToName(uri string) string {
	_, name, ok := strings.Cut(uri, "/")
	if !ok {
		return uri
	}
	return name
}
