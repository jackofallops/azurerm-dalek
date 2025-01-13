package cleaners

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/go-azure-helpers/resourcemanager/commonids"
	"github.com/hashicorp/go-azure-sdk/resource-manager/recoveryservices/2023-02-01/vaults"
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

	vaultsList, err := vaultsClient.ListBySubscriptionIdComplete(ctx, subscriptionId)
	if err != nil {
		return fmt.Errorf("listing Recovery Services Vault for %s: %+v", subscriptionId, err)
	}

	for _, vault := range vaultsList.Items {
		if vault.Id == nil {
			continue
		}

		vaultId, err := vaults.ParseVaultID(*vault.Id)
		if err != nil {
			log.Printf("[DEBUG] parsing id %q: %+v", *vault.Id, err)
			continue
		}

		if _, err := vaultsClient.Delete(ctx, *vaultId); err != nil {
			log.Printf("[DEBUG] deleting %q: %+v", vaultId.ID(), err)
			continue
		}

		return fmt.Errorf("Just checking the one")
	}

	return nil
}
