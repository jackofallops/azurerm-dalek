package cleaners

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/go-azure-helpers/resourcemanager/commonids"
	"github.com/hashicorp/go-azure-sdk/resource-manager/recoveryservices/2024-10-01/vaults"
	"github.com/hashicorp/go-azure-sdk/resource-manager/recoveryservicesbackup/2024-10-01/backupprotecteditems"
	"github.com/tombuildsstuff/azurerm-dalek/clients"
	"github.com/tombuildsstuff/azurerm-dalek/dalek/options"
)

type deleteRecoveryServicesVaultSubscriptionCleaner struct{}

var _ SubscriptionCleaner = deleteRecoveryServicesVaultSubscriptionCleaner{}

func (p deleteRecoveryServicesVaultSubscriptionCleaner) Name() string {
	return "Removing Recovery Services Vault"
}

func (p deleteRecoveryServicesVaultSubscriptionCleaner) Cleanup(ctx context.Context, subscriptionId commonids.SubscriptionId, client *clients.AzureClient, opts options.Options) error {
	vaultsClient := client.ResourceManager.RecoveryServicesVaultClient
	protectedItemsClient := client.ResourceManager.RecoveryServicesProtectedItemClient
	backupProtectedItemsClient := client.ResourceManager.RecoveryServicesBackupProtectedItemsClient

	vaultsList, err := vaultsClient.ListBySubscriptionIdComplete(ctx, subscriptionId)
	if err != nil {
		return fmt.Errorf("listing Recovery Services Vault for %s: %+v", subscriptionId, err)
	}

	for _, vault := range vaultsList.Items {
		if vault.Id == nil {
			continue
		}

		backupItemsVaultId, err := backupprotecteditems.ParseVaultID(*vault.Id)
		if err != nil {
			log.Printf("[DEBUG] parsing id %q: %+v", *vault.Id, err)
			continue
		}

		if backupItemsVaultId == nil {
			return fmt.Errorf("wtfff")
		}

		_, err = backupProtectedItemsClient.List(ctx, *backupItemsVaultId, backupprotecteditems.ListOperationOptions{})
		if err != nil {
			log.Printf("listing Backup Protected Items for %q: %+v", backupItemsVaultId.ID(), err)
			continue
		}
		return fmt.Errorf("Here")

		// protectedItemsClient.Get()

		vaultId, err := vaults.ParseVaultID(*vault.Id)
		if err != nil {
			log.Printf("[DEBUG] parsing id %q: %+v", *vault.Id, err)
			continue
		}

		if err := vaultsClient.DeleteThenPoll(ctx, *vaultId); err != nil {
			// todo remove this return
			return fmt.Errorf("deleting %q: %+v", vaultId.ID(), err)
			log.Printf("[DEBUG] deleting %q: %+v", vaultId.ID(), err)
			continue
		}
	}

	return nil
}
