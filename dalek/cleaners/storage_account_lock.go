package cleaners

import (
	"context"
	"log"

	"github.com/hashicorp/go-azure-helpers/resourcemanager/commonids"
	"github.com/hashicorp/go-azure-sdk/resource-manager/resources/2020-05-01/managementlocks"
	"github.com/jackofallops/azurerm-dalek/clients"
	"github.com/jackofallops/azurerm-dalek/dalek/options"
)

var _ ResourceGroupCleaner = removeLocksFromStorageAccountCleaner{}

type removeLocksFromStorageAccountCleaner struct {
}

func (removeLocksFromStorageAccountCleaner) Name() string {
	return "Removing Locks from Storage Accounts.."
}

func (removeLocksFromStorageAccountCleaner) Cleanup(ctx context.Context, id commonids.ResourceGroupId, client *clients.AzureClient, opts options.Options) error {
	var storageAccountScopeId commonids.ScopeId
	locks, err := client.ResourceManager.LocksClient.ListByScopeComplete(ctx, storageAccountScopeId, managementlocks.DefaultListByScopeOperationOptions())
	if err != nil {
		log.Printf("[DEBUG] Error obtaining Storage Account Locks : %+v", err)
	}

	if items := locks.Items; items != nil {
		for _, lock := range items {
			if lock.Id == nil {
				continue
			}
			lockId, err := managementlocks.ParseScopedLockID(*lock.Id)
			if err != nil {
				log.Printf("[ERROR] Parsing Scoped Lock ID %q: %+v", *lock.Id, err)
				continue
			}
			if _, err := client.ResourceManager.LocksClient.DeleteByScope(ctx, *lockId); err != nil {
				log.Printf("[DEBUG]   Unable to delete lock %s on storage account %q. %v", *lock.Name, id.ResourceGroupName, err)
			}
			log.Printf("[DEBUG] Deleted lock '%s' on storage account %q", *lock.Name, storageAccountScopeId)
		}
	}
	return nil
}

func (removeLocksFromStorageAccountCleaner) ResourceTypes() []string {
	return []string{
		"Microsoft.Storage/storageaccounts",
	}
}
