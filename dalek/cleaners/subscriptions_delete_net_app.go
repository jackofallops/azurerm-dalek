package cleaners

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/go-azure-helpers/lang/pointer"
	"github.com/hashicorp/go-azure-helpers/resourcemanager/commonids"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2025-03-01/capacitypools"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2025-03-01/netappaccounts"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2025-03-01/volumes"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2025-03-01/volumesreplication"
	"github.com/hashicorp/go-azure-sdk/resource-manager/resourcegraph/2024-04-01/resources"
	"github.com/hashicorp/go-azure-sdk/resource-manager/vmware/2024-09-01/datastores"
	"github.com/jackofallops/azurerm-dalek/clients"
	"github.com/jackofallops/azurerm-dalek/dalek/options"
)

type deleteNetAppSubscriptionCleaner struct{}

var _ SubscriptionCleaner = deleteNetAppSubscriptionCleaner{}

func (p deleteNetAppSubscriptionCleaner) Name() string {
	return "Removing Net App"
}

func (p deleteNetAppSubscriptionCleaner) Cleanup(ctx context.Context, subscriptionId commonids.SubscriptionId, client *clients.AzureClient, opts options.Options) error {
	netAppAccountClient := client.ResourceManager.NetAppAccountClient
	netAppCapcityPoolClient := client.ResourceManager.NetAppCapacityPoolClient
	netAppVolumeClient := client.ResourceManager.NetAppVolumeClient
	netAppVolumeReplicationClient := client.ResourceManager.NetAppVolumeReplicationClient

	errs := make([]error, 0)

	accountLists, err := netAppAccountClient.AccountsListBySubscription(ctx, subscriptionId)
	if err != nil {
		return fmt.Errorf("listing NetApp Accounts for %s: %+v", subscriptionId, err)
	}

	if accountLists.Model == nil {
		return fmt.Errorf("listing NetApp Accounts: model was nil")
	}

	for _, account := range *accountLists.Model {
		if account.Id == nil {
			continue
		}

		accountIdForCapacityPool, err := capacitypools.ParseNetAppAccountID(*account.Id)
		if err != nil {
			return err
		}

		if !strings.HasPrefix(accountIdForCapacityPool.ResourceGroupName, opts.Prefix) {
			log.Printf("[DEBUG] Not deleting %q as it does not match target RG prefix %q", *accountIdForCapacityPool, opts.Prefix)
			continue
		}

		accountId, err := netappaccounts.ParseNetAppAccountID(*account.Id)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		canDeleteAccount := true

		capacityPoolList, err := netAppCapcityPoolClient.PoolsListComplete(ctx, *accountIdForCapacityPool)
		if err != nil {
			errs = append(errs, fmt.Errorf("listing NetApp Capacity Pools for %s: %+v", accountIdForCapacityPool, err))
			canDeleteAccount = false
		}

		for _, capacityPool := range capacityPoolList.Items {
			if capacityPool.Id == nil {
				continue
			}

			capacityPoolId, err := capacitypools.ParseCapacityPoolID(*capacityPool.Id)
			if err != nil {
				errs = append(errs, err)
				canDeleteAccount = false
				continue
			}

			capacityPoolForVolumesId, err := volumes.ParseCapacityPoolID(*capacityPool.Id)
			if err != nil {
				errs = append(errs, err)
				canDeleteAccount = false
				continue
			}

			canDeletePool := true

			volumeList, err := netAppVolumeClient.ListComplete(ctx, *capacityPoolForVolumesId)
			if err != nil {
				errs = append(errs, fmt.Errorf("listing NetApp Volumes for %s: %+v", capacityPoolForVolumesId, err))
				canDeletePool = false
			}

			for _, volume := range volumeList.Items {
				if volume.Id == nil {
					continue
				}

				volumeId, err := volumes.ParseVolumeID(*volume.Id)
				if err != nil {
					errs = append(errs, err)
					canDeletePool = false
					continue
				}

				volumeReplicationId, err := volumesreplication.ParseVolumeID(*volume.Id)
				if err != nil {
					errs = append(errs, err)
					canDeletePool = false
					continue
				}

				hasReplication := volume.Properties.DataProtection != nil &&
					volume.Properties.DataProtection.Replication != nil &&
					volume.Properties.DataProtection.Replication.EndpointType != nil &&
					strings.EqualFold(string(*volume.Properties.DataProtection.Replication.EndpointType), string(volumes.EndpointTypeDst))

				if hasReplication {
					if !opts.ActuallyDelete {
						log.Printf("[DEBUG] Would have deleted replication for %s..", volumeReplicationId)
					} else {
						if err := netAppVolumeReplicationClient.VolumesDeleteReplicationThenPoll(ctx, *volumeReplicationId); err != nil {
							errs = append(errs, fmt.Errorf("deleting replication for %s: %+v", volumeReplicationId, err))
						}
					}
				}

				isAvsDataStore := volume.Properties.AvsDataStore != nil &&
					strings.EqualFold(string(*volume.Properties.AvsDataStore), string(volumes.AvsDataStoreEnabled))

				if isAvsDataStore {
					datastoreIds, err := findAvsDatastoresForVolume(ctx, client, volumeId.ID(), subscriptionId.SubscriptionId)
					if err != nil {
						errs = append(errs, fmt.Errorf("finding AVS datastores for volume %s: %+v", volumeId, err))
					} else {
						for _, dsId := range datastoreIds {
							if !opts.ActuallyDelete {
								log.Printf("[DEBUG] Would have deleted AVS datastore %s for NetApp volume %s..", dsId, volumeId)
								continue
							}
							log.Printf("[DEBUG] Deleting AVS datastore %s for NetApp volume %s..", dsId, volumeId)
							if err := client.ResourceManager.VMwareDatastoresClient.DeleteThenPoll(ctx, dsId); err != nil {
								errs = append(errs, fmt.Errorf("deleting AVS datastore %s for NetApp volume %s: %+v", dsId, volumeId, err))
							}
						}
					}
				}

				if !opts.ActuallyDelete {
					log.Printf("[DEBUG] Would have deleted %s..", volumeId)
					continue
				}

				forceDelete := true
				if err = netAppVolumeClient.DeleteThenPoll(ctx, *volumeId, volumes.DeleteOperationOptions{ForceDelete: &forceDelete}); err != nil {
					errs = append(errs, fmt.Errorf("[DEBUG] Unable to delete %s: %+v", volumeId, err))
					canDeletePool = false
					continue
				}
			}

			if !canDeletePool {
				log.Printf("[DEBUG] Skipping deletion of NetApp Capacity Pool %s because one or more child volumes failed to delete", capacityPoolId)
				canDeleteAccount = false
				continue
			}

			if !opts.ActuallyDelete {
				log.Printf("[DEBUG] Would have deleted %s..", capacityPoolId)
				continue
			}

			if err = netAppCapcityPoolClient.PoolsDeleteThenPoll(ctx, *capacityPoolId); err != nil {
				errs = append(errs, fmt.Errorf("[DEBUG] Unable to delete %s: %+v", capacityPoolId, err))
				canDeleteAccount = false
				continue
			}
		}

		if !canDeleteAccount {
			log.Printf("[DEBUG] Skipping deletion of NetApp Account %s because one or more child capacity pools failed to delete", accountId)
			continue
		}

		if !opts.ActuallyDelete {
			log.Printf("[DEBUG] Would have deleted %s..", accountId)
			continue
		}

		if err = netAppAccountClient.AccountsDeleteThenPoll(ctx, *accountId); err != nil {
			errs = append(errs, fmt.Errorf("[DEBUG] Unable to delete %s: %+v", accountId, err))
			continue
		}
	}

	return errors.Join(errs...)
}

func findAvsDatastoresForVolume(ctx context.Context, client *clients.AzureClient, volumeId string, subscriptionId string) ([]datastores.DataStoreId, error) {
	if client == nil || client.ResourceManager.ResourceGraphClient == nil {
		return nil, fmt.Errorf("ResourceGraphClient is not configured")
	}

	query := fmt.Sprintf(`
resources
| where type =~ "microsoft.avs/privateclouds/clusters/datastores"
| where properties.netAppVolume.id =~ '%s'
| project id
| sort by (tolower(tostring(id))) asc
`, volumeId)

	payload := resources.QueryRequest{
		Options: &resources.QueryRequestOptions{
			Top: pointer.To(int64(1000)),
		},
		Query: query,
		Subscriptions: &[]string{
			subscriptionId,
		},
	}

	resp, err := client.ResourceManager.ResourceGraphClient.Resources(ctx, payload)
	if err != nil {
		return nil, fmt.Errorf("querying AVS datastores for volume %q: %+v", volumeId, err)
	}

	if resp.Model == nil || resp.Model.Data == nil {
		return nil, nil
	}

	itemsRaw, ok := resp.Model.Data.([]interface{})
	if !ok {
		return nil, fmt.Errorf("expected ARG response data to be []interface{}, got %T", resp.Model.Data)
	}

	datastoreIds := make([]datastores.DataStoreId, 0, len(itemsRaw))
	for index, itemRaw := range itemsRaw {
		item, ok := itemRaw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("expected item %d to be map[string]interface{}, got %T", index, itemRaw)
		}
		idVal, ok := item["id"]
		if !ok {
			continue
		}
		idStr, ok := idVal.(string)
		if !ok {
			continue
		}
		datastoreId, err := datastores.ParseDataStoreIDInsensitively(idStr)
		if err != nil {
			return nil, fmt.Errorf("parsing datastore ID %q: %+v", idStr, err)
		}
		datastoreIds = append(datastoreIds, *datastoreId)
	}

	return datastoreIds, nil
}
