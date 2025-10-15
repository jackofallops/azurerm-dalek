package cleaners

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-11-01/snapshotpolicy"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-11-01/snapshots"
	"github.com/hashicorp/go-azure-sdk/sdk/client/pollers"
	"github.com/jackofallops/azurerm-dalek/clients"
	"github.com/jackofallops/azurerm-dalek/dalek/options"
)

func deleteSnapshots(ctx context.Context, volumeIdForSnapshots snapshots.VolumeId, client *clients.AzureClient, opts options.Options) error {
	snapshotClient := client.ResourceManager.NetAppSnapshotClient
	resp, err := snapshotClient.List(ctx, volumeIdForSnapshots)
	if err != nil || resp.Model == nil || resp.Model.Value == nil {
		return fmt.Errorf("listing NetApp Snapshots for %s", volumeIdForSnapshots)
	}
	if len(*resp.Model.Value) == 0 {
		return nil
	}
	log.Printf("[DEBUG] Found %d NetApp Snapshots in %s", len(*resp.Model.Value), volumeIdForSnapshots.NetAppAccountName)
	for _, snapshot := range *resp.Model.Value {
		if snapshot.Id == nil {
			continue
		}
		snapshotID, err := snapshots.ParseSnapshotID(*snapshot.Id)
		if err != nil {
			return err
		}
		if !opts.ActuallyDelete {
			log.Printf("[DEBUG] Would have deleted %s", snapshotID.String())
			continue
		}
		response, err := snapshotClient.Delete(ctx, *snapshotID)
		if err != nil {
			return err
		}
		pollerType, err := NewLROPoller(&lroClientAdapter{inner: snapshotClient.Client}, response.HttpResponse)
		if err != nil {
			return fmt.Errorf("creating poller for %s: %+v", snapshotID, err)
		}
		if pollerType != nil {
			poller := pollers.NewPoller(pollerType, 0, 10)
			if err := poller.PollUntilDone(ctx); err != nil {
				return fmt.Errorf("polling delete operation for %s: %+v", snapshotID, err)
			}
		}
		log.Printf("[DEBUG] Deleted %s", snapshotID)
	}
	return nil
}

func deleteSnapshotPolicies(ctx context.Context, accountIdForSnapshots snapshotpolicy.NetAppAccountId, client *clients.AzureClient, opts options.Options) error {
	snapshotPolicyClient := client.ResourceManager.NetAppSnapshotPolicyClient
	resp, err := snapshotPolicyClient.SnapshotPoliciesList(ctx, accountIdForSnapshots)
	if err != nil {
		return fmt.Errorf("listing NetApp Snapshot Policies for %s: %+v", accountIdForSnapshots, err)
	}
	if resp.Model == nil {
		return fmt.Errorf("listing NetApp Snapshot Policies for %s: resp.Model is nil", accountIdForSnapshots)
	}
	if resp.Model.Value == nil {
		return fmt.Errorf("listing NetApp Snapshot Policies for %s: resp.Model.Value is nil", accountIdForSnapshots)
	}
	log.Printf("[DEBUG] Found %d NetApp Snapshot Policies in %s", len(*resp.Model.Value), accountIdForSnapshots.NetAppAccountName)
	for _, snapshotPolicy := range *resp.Model.Value {
		if snapshotPolicy.Id == nil {
			continue
		}
		policyID, err := snapshotpolicy.ParseSnapshotPolicyID(*snapshotPolicy.Id)
		if err != nil {
			return err
		}
		if !opts.ActuallyDelete {
			log.Printf("[DEBUG] Would have deleted %s", policyID.String())
			continue
		}
		response, err := snapshotPolicyClient.SnapshotPoliciesDelete(ctx, *policyID)
		if err != nil {
			return err
		}
		pollerType, err := NewLROPoller(&lroClientAdapter{inner: snapshotPolicyClient.Client}, response.HttpResponse)
		if err != nil {
			return fmt.Errorf("creating poller for %s: %+v", policyID, err)
		}
		if pollerType != nil {
			poller := pollers.NewPoller(pollerType, 0, 10)
			if err := poller.PollUntilDone(ctx); err != nil {
				return fmt.Errorf("polling delete operation for %s: %+v", policyID, err)
			}
		}
		log.Printf("[DEBUG] Deleted %s", policyID)
	}
	return nil
}
