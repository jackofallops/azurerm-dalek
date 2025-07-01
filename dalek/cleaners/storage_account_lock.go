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
	return "Storage Account Locks"
}

func (removeLocksFromStorageAccountCleaner) Cleanup(ctx context.Context, id commonids.ResourceGroupId, client *clients.AzureClient, opts options.Options) error {
	storageAccounts, err := client.ResourceManager.StorageAccountsClient.ListByResourceGroupComplete(ctx, id)
	if err != nil {
		log.Printf("[DEBUG] Error listing storage accounts in resource group %q: %+v", id.ResourceGroupName, err)
		return err
	}
	for _, account := range storageAccounts.Items {
		if account.Id == nil {
			continue
		}
		accountScopeId := commonids.NewScopeID(*account.Id)
		result, err := client.ResourceManager.LocksClient.ListByScopeComplete(ctx, accountScopeId, managementlocks.DefaultListByScopeOperationOptions())
		if err != nil {
			log.Printf("[DEBUG] Error checking locks for storage account %s: %+v", *account.Id, err)
			continue
		}
		if items := result.Items; items != nil {
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
				log.Printf("[DEBUG] Deleted lock '%s' on storage account %q", *lock.Name, accountScopeId)
			}
		}
	}
	return nil
}

func (removeLocksFromStorageAccountCleaner) ResourceTypes() []string {
	return []string{
		"Microsoft.Storage/storageaccounts",
	}
}
