package cleaners

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/go-azure-helpers/lang/pointer"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-11-01/snapshots"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-11-01/volumes"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-11-01/volumesreplication"
	"github.com/hashicorp/go-azure-sdk/sdk/client/pollers"
	"github.com/jackofallops/azurerm-dalek/clients"
	"github.com/jackofallops/azurerm-dalek/dalek/options"
)

func deleteVolumes(ctx context.Context, capacityPoolIdForVolumes volumes.CapacityPoolId, client *clients.AzureClient, opts options.Options) error {
	netAppVolumeClient := client.ResourceManager.NetAppVolumeClient
	volumeList, err := netAppVolumeClient.ListComplete(ctx, capacityPoolIdForVolumes)
	if err != nil {
		return fmt.Errorf("listing NetApp Volumes for %s: %+v", capacityPoolIdForVolumes, err)
	}
	log.Printf("[DEBUG] Found %d NetApp Volumes in %s", len(volumeList.Items), capacityPoolIdForVolumes.NetAppAccountName)
	for _, volume := range volumeList.Items {
		if volume.Id == nil {
			continue
		}

		volumeIdForSnapshots, err := snapshots.ParseVolumeID(pointer.From(volume.Id))
		if err != nil {
			return err
		}
		if err := deleteSnapshots(ctx, pointer.From(volumeIdForSnapshots), client, opts); err != nil {
			return err
		}

		volumeIdForReplication, err := volumesreplication.ParseVolumeID(*volume.Id)
		if err != nil {
			return err
		}
		if !opts.ActuallyDelete {
			log.Printf("[DEBUG] Would have deleted %s", volumeIdForReplication)
		} else {
			netAppVolumeReplicationClient := client.ResourceManager.NetAppVolumeReplicationClient
			if _, err := netAppVolumeReplicationClient.VolumesDeleteReplication(ctx, *volumeIdForReplication); err != nil {
				return err
			}
			log.Printf("[DEBUG] Deleted replication for %s", volumeIdForReplication)
		}

		volumeId, err := volumes.ParseVolumeID(*volume.Id)
		if err != nil {
			return err
		}
		if !opts.ActuallyDelete {
			log.Printf("[DEBUG] Would have deleted %s", volumeId)
			continue
		}

		forceDelete := true
		response, err := netAppVolumeClient.Delete(ctx, *volumeId, volumes.DeleteOperationOptions{ForceDelete: &forceDelete})
		if err != nil {
			return err
		}
		pollerType, err := NewLROPoller(&lroClientAdapter{inner: netAppVolumeClient.Client}, response.HttpResponse)
		if err != nil {
			return fmt.Errorf("creating poller for %s: %+v", volumeId, err)
		}
		if pollerType != nil {
			poller := pollers.NewPoller(pollerType, 0, 10)
			err := poller.PollUntilDone(ctx)
			if err != nil {
				return fmt.Errorf("polling delete operation for %s: %+v", volumeId, err)
			}
		}
		log.Printf("[DEBUG] Deleted %s", volumeId)
	}

	return nil
}
