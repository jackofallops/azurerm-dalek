package cleaners

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/go-azure-helpers/resourcemanager/commonids"
	"github.com/hashicorp/go-azure-sdk/resource-manager/storagesync/2020-03-01/cloudendpointresource"
	"github.com/hashicorp/go-azure-sdk/resource-manager/storagesync/2020-03-01/registeredserverresource"
	"github.com/hashicorp/go-azure-sdk/resource-manager/storagesync/2020-03-01/storagesyncservicesresource"
	"github.com/hashicorp/go-azure-sdk/resource-manager/storagesync/2020-03-01/syncgroupresource"
	"github.com/jackofallops/azurerm-dalek/clients"
	"github.com/jackofallops/azurerm-dalek/dalek/options"
)

type deleteStorageSyncSubscriptionCleaner struct{}

var _ SubscriptionCleaner = deleteStorageSyncSubscriptionCleaner{}

func (p deleteStorageSyncSubscriptionCleaner) Name() string {
	return "Removing Storage Sync"
}

func (p deleteStorageSyncSubscriptionCleaner) Cleanup(ctx context.Context, subscriptionId commonids.SubscriptionId, client *clients.AzureClient, opts options.Options) error {
	storageSyncClient := client.ResourceManager.StorageSyncClient
	storageSyncGroupClient := client.ResourceManager.StorageSyncGroupClient
	storageSyncCloudEndpointClient := client.ResourceManager.StorageSyncCloudEndpointClient
	storageSyncRegisteredServerClient := client.ResourceManager.StorageSyncRegisteredServerClient

	errs := make([]error, 0)

	storageSyncList, err := storageSyncClient.StorageSyncServicesListBySubscription(ctx, subscriptionId)
	if err != nil {
		return fmt.Errorf("listing storage syncs: %+v", err)
	}

	if storageSyncList.Model == nil || storageSyncList.Model.Value == nil {
		return fmt.Errorf("listing storage syncs: model/value was nil")
	}

	for _, storageSync := range *storageSyncList.Model.Value {
		if storageSync.Id == nil {
			continue
		}

		id, err := storagesyncservicesresource.ParseStorageSyncServiceID(*storageSync.Id)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		if !strings.HasPrefix(id.ResourceGroupName, opts.Prefix) {
			log.Printf("[DEBUG] Not deleting %q as it does not match target RG prefix %q", *id, opts.Prefix)
			continue
		}

		// Storage Sync Registered Servers Cleanup

		registeredServers, err := storageSyncRegisteredServerClient.RegisteredServersListByStorageSyncService(ctx, registeredserverresource.StorageSyncServiceId(*id))
		if err != nil {
			errs = append(errs, err)
			continue
		}

		if registeredServers.Model == nil || registeredServers.Model.Value == nil {
			continue
		}

		for _, s := range *registeredServers.Model.Value {
			if s.Id == nil {
				continue
			}

			registeredServerID, err := registeredserverresource.ParseRegisteredServerID(*s.Id)
			if err != nil {
				errs = append(errs, err)
				continue
			}

			if !opts.ActuallyDelete {
				log.Printf("[DEBUG] Would have deleted %s", registeredServerID)
				continue
			}

			if err := storageSyncRegisteredServerClient.RegisteredServersDeleteThenPoll(ctx, *registeredServerID); err != nil {
				errs = append(errs, fmt.Errorf("deleting %s: %w", registeredServerID, err))
			}
		}

		// Storage Sync Group Cleanup

		if !opts.ActuallyDelete {
			log.Printf("[DEBUG] Would have deleted %s..", id)
			continue
		}

		groupList, err := storageSyncGroupClient.SyncGroupsListByStorageSyncService(ctx, syncgroupresource.StorageSyncServiceId(*id))
		if err != nil {
			errs = append(errs, fmt.Errorf("listing storage sync groups for %s: %+v", id, err))
		}

		if groupList.Model == nil || groupList.Model.Value == nil {
			continue
		}

		for _, group := range *groupList.Model.Value {
			if group.Id == nil {
				continue
			}

			groupIdForCloudEndpoint, err := cloudendpointresource.ParseSyncGroupID(*group.Id)
			if err != nil {
				errs = append(errs, err)
				continue
			}

			if !opts.ActuallyDelete {
				log.Printf("[DEBUG] Would have deleted %s..", groupIdForCloudEndpoint)
				continue
			}

			cloudEndpointList, err := storageSyncCloudEndpointClient.CloudEndpointsListBySyncGroup(ctx, *groupIdForCloudEndpoint)
			if err != nil {
				errs = append(errs, fmt.Errorf("listing cloud endpoints for %s: %+v", groupIdForCloudEndpoint, err))
				continue
			}

			if cloudEndpointList.Model == nil || cloudEndpointList.Model.Value == nil {
				continue
			}

			for _, endpoint := range *cloudEndpointList.Model.Value {
				if endpoint.Id == nil {
					continue
				}

				endpointId, err := cloudendpointresource.ParseCloudEndpointID(*endpoint.Id)
				if err != nil {
					errs = append(errs, err)
					continue
				}

				if !opts.ActuallyDelete {
					log.Printf("[DEBUG] Would have deleted %s..", endpointId)
					continue
				}

				if err = storageSyncCloudEndpointClient.CloudEndpointsDeleteThenPoll(ctx, *endpointId); err != nil {
					errs = append(errs, fmt.Errorf("deleting %s: %+v", endpointId, err))
					continue
				}
			}

			groupId, err := syncgroupresource.ParseSyncGroupID(*group.Id)
			if err != nil {
				errs = append(errs, err)
				continue
			}

			if _, err = storageSyncGroupClient.SyncGroupsDelete(ctx, *groupId); err != nil {
				errs = append(errs, fmt.Errorf("deleting %s: %+v", groupId, err))
			}
		}

		// Storage Sync Service Cleanup

		storageSyncId, err := storagesyncservicesresource.ParseStorageSyncServiceID(*storageSync.Id)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if err = storageSyncClient.StorageSyncServicesDeleteThenPoll(ctx, *storageSyncId); err != nil {
			errs = append(errs, fmt.Errorf("deleting %s: %+v", storageSyncId, err))
		}
	}

	return errors.Join(errs...)
}
