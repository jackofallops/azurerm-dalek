package cleaners

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/go-azure-helpers/lang/pointer"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-11-01/capacitypools"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-11-01/snapshotpolicy"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-11-01/volumes"
	"github.com/hashicorp/go-azure-sdk/sdk/client/pollers"
	"github.com/jackofallops/azurerm-dalek/clients"
	"github.com/jackofallops/azurerm-dalek/dalek/options"
)

func deleteCapacityPools(ctx context.Context, accountIdForCapacityPool capacitypools.NetAppAccountId, client *clients.AzureClient, opts options.Options) error {
	netAppCapacityPoolClient := client.ResourceManager.NetAppCapacityPoolClient
	capacityPoolList, err := netAppCapacityPoolClient.PoolsListComplete(ctx, accountIdForCapacityPool)
	if err != nil {
		return fmt.Errorf("listing NetApp Capacity Pools for %s: %+v", accountIdForCapacityPool, err)
	}

	log.Printf("[DEBUG] Found %d NetApp Capacity Pools in %s", len(capacityPoolList.Items), accountIdForCapacityPool.NetAppAccountName)
	for _, capacityPool := range capacityPoolList.Items {
		if capacityPool.Id == nil {
			continue
		}
		capacityPoolForVolumesId, err := volumes.ParseCapacityPoolID(pointer.From(capacityPool.Id))
		if err != nil {
			return err
		}
		if err := deleteVolumes(ctx, pointer.From(capacityPoolForVolumesId), client, opts); err != nil {
			return err
		}
		accountIdForSnapshots, err := snapshotpolicy.ParseNetAppAccountID(accountIdForCapacityPool.ID())
		if err != nil {
			return err
		}
		if err := deleteSnapshotPolicies(ctx, pointer.From(accountIdForSnapshots), client, opts); err != nil {
			return err
		}
		capacityPoolId, err := capacitypools.ParseCapacityPoolID(*capacityPool.Id)
		if err != nil {
			return err
		}

		if !opts.ActuallyDelete {
			log.Printf("[DEBUG] Would have deleted %s", capacityPoolId)
			continue
		}
		response, err := netAppCapacityPoolClient.PoolsDelete(ctx, *capacityPoolId)
		if err != nil {
			return err
		}
		pollerType, err := NewLROPoller(&lroClientAdapter{inner: netAppCapacityPoolClient.Client}, response.HttpResponse)
		if err != nil {
			return fmt.Errorf("creating poller for %s: %+v", capacityPoolId, err)
		}
		if pollerType != nil {
			poller := pollers.NewPoller(pollerType, 0, 10)
			err := poller.PollUntilDone(ctx)
			if err != nil {
				return fmt.Errorf("polling delete operation for %s: %+v", capacityPoolId, err)
			}
		}

		poolList, err := netAppCapacityPoolClient.PoolsListComplete(ctx, accountIdForCapacityPool)
		if err == nil {
			for _, pool := range poolList.Items {
				if pool.Id != nil && *pool.Id == capacityPoolId.String() {
					return fmt.Errorf("[ERROR] Capacity pool %s still exists after delete attempt", capacityPoolId.String())
				}
			}
		}
		log.Printf("[DEBUG] Deleted %s", capacityPoolId)
	}
	return nil
}
