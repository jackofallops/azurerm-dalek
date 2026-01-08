package cleaners

import (
	"context"
	"log"
	"time"

	"github.com/hashicorp/go-azure-helpers/lang/response"
	"github.com/hashicorp/go-azure-helpers/resourcemanager/commonids"
	"github.com/hashicorp/go-azure-sdk/resource-manager/resources/2020-05-01/managementlocks"
	"github.com/hashicorp/go-azure-sdk/sdk/client/pollers"
	"github.com/jackofallops/azurerm-dalek/clients"
	"github.com/jackofallops/azurerm-dalek/dalek/options"
)

var _ ResourceGroupCleaner = removeLocksFromResourceGroupCleaner{}

type removeLocksFromResourceGroupCleaner struct{}

func (removeLocksFromResourceGroupCleaner) Name() string {
	return "Removing Locks.."
}

func (removeLocksFromResourceGroupCleaner) Cleanup(ctx context.Context, id commonids.ResourceGroupId, client *clients.AzureClient, opts options.Options) error {
	locks, err := client.ResourceManager.LocksClient.ListAtResourceGroupLevel(ctx, id, managementlocks.DefaultListAtResourceGroupLevelOperationOptions())
	if err != nil {
		log.Printf("[DEBUG] Error obtaining Resource Group Locks : %+v", err)
	}

	if model := locks.Model; model != nil {
		for _, lock := range *model {
			if lock.Id == nil {
				log.Printf("[DEBUG]   Lock with nil id on %q", id.ResourceGroupName)
				continue
			}
			lockId, err := managementlocks.ParseScopedLockID(*lock.Id)
			if err != nil {
				log.Printf("[ERROR] Parsing Scoped Lock ID %q: %+v", *lock.Id, err)
				continue
			}

			if lock.Name == nil {
				log.Printf("[DEBUG]   Lock %s with nil name on %q", id, id.ResourceGroupName)
				continue
			}

			log.Printf("[DEBUG]   Attemping to remove lock %s from: %s", id, id.ResourceGroupName)

			if _, err := client.ResourceManager.LocksClient.DeleteByScope(ctx, *lockId); err != nil {
				log.Printf("[DEBUG]   Unable to delete lock %s on resource group %q", *lock.Name, id.ResourceGroupName)
				continue
			}

			// Deletion of locks has been observed to be delayed (asynch) for some scopes.
			// Use a simple poller to wait for lock removal, otherwise RG deletion will fail if any delay occurs
			log.Printf("[DEBUG]   Polling for lock deletion of: %s", *lockId)
			pollerType := lockDeletePoller{
				client: client.ResourceManager.LocksClient,
				lockId: *lockId,
			}
			poller := pollers.NewPoller(pollerType, 5*time.Second, pollers.DefaultNumberOfDroppedConnectionsToAllow)
			if err := poller.PollUntilDone(ctx); err != nil {
				log.Printf("[ERROR] Polling for deletion is broken for lock %s: %+v", *lockId, err)
				continue
			}
			log.Printf("[DEBUG] Lock delete is complete for %s", *lockId)
		}
	}
	return nil
}

func (removeLocksFromResourceGroupCleaner) ResourceTypes() []string {
	return []string{
		"Microsoft.Authorization/locks",
	}
}

type lockDeletePoller struct {
	client *managementlocks.ManagementLocksClient
	lockId managementlocks.ScopedLockId
}

func (p lockDeletePoller) Poll(ctx context.Context) (*pollers.PollResult, error) {
	result, err := p.client.GetByScope(ctx, p.lockId)
	if err != nil {
		if response.WasNotFound(result.HttpResponse) {
			return &pollers.PollResult{
				Status: pollers.PollingStatusSucceeded,
			}, nil
		}
		return nil, pollers.PollingFailedError{
			Message: err.Error(),
		}
	}

	return &pollers.PollResult{
		Status:       pollers.PollingStatusInProgress,
		PollInterval: 5 * time.Second,
	}, nil
}
