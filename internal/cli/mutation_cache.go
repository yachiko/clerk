package cli

import (
	"context"
	"time"

	"github.com/yachiko/clerk/internal/aws"
	"github.com/yachiko/clerk/internal/cache"
	"github.com/yachiko/clerk/internal/config"
)

func reconcileSecretCache(ctx context.Context, cfg *config.Config, resolved *aws.ResolvedContext, client *aws.SecretsManagerClient, name string) {
	manager, err := cache.NewManagerForBackend(cfg, resolved.Partition, resolved.Region, resolved.AccountID, aws.BackendSecretsManager)
	if err != nil {
		return
	}
	metadata, err := client.DescribeSecret(ctx, name)
	if err != nil {
		return
	}
	modified := metadata.LastChangedDate
	if modified == nil {
		modified = metadata.CreatedDate
	}
	entry := cache.CacheEntry{Identity: metadata.Identity, Name: metadata.Name, Type: "Secret", Tags: metadata.Tags, TagsFetchedAt: time.Now(), TagsComplete: true}
	if modified != nil {
		entry.LastModifiedDate = *modified
	}
	_ = manager.Update(entry)
}

func reconcileParameterTags(ctx context.Context, cfg *config.Config, client *aws.Client, name string) {
	metadata, err := client.GetParameterMetadata(ctx, name)
	if err != nil {
		return
	}
	tags, err := client.GetParameterTags(ctx, name)
	if err != nil {
		return
	}
	manager, err := cache.NewManagerForBackend(cfg, client.GetPartition(), client.GetRegion(), client.GetAccountID(), aws.BackendSSM)
	if err != nil {
		return
	}
	_ = manager.Update(cache.CacheEntry{Name: metadata.Name, Type: metadata.Type, Version: metadata.Version, LastModifiedDate: metadata.LastModifiedDate, Tags: tags, TagsFetchedAt: time.Now(), TagsComplete: true})
}
