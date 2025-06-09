package cleaners

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/go-azure-helpers/lang/pointer"
	"github.com/hashicorp/go-azure-helpers/resourcemanager/commonids"
	"github.com/hashicorp/go-azure-sdk/resource-manager/recoveryservices/2024-10-01/vaults"
	"github.com/hashicorp/go-azure-sdk/resource-manager/recoveryservicesbackup/2024-10-01/backupprotecteditems"
	"github.com/hashicorp/go-azure-sdk/resource-manager/recoveryservicesbackup/2024-10-01/protecteditems"
	"github.com/jackofallops/azurerm-dalek/clients"
	"github.com/jackofallops/azurerm-dalek/dalek/options"
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

		vaultId, err := vaults.ParseVaultID(*vault.Id)
		if err != nil {
			log.Printf("[DEBUG] parsing id %q: %+v", *vault.Id, err)
			continue
		}

		if !strings.HasPrefix(strings.ToLower(vaultId.ResourceGroupName), strings.ToLower(opts.Prefix)) {
			log.Printf("[DEBUG] Not deleting %q as it does not match target RG prefix %q", vaultId.ResourceGroupName, opts.Prefix)
			continue
		}

		// Update the vault to be mutable
		isSoftDeleteEnabled := false
		isImmutable := false
		isMUA := false
		if vault.Properties != nil && vault.Properties.SecuritySettings != nil && vault.Properties.SecuritySettings.SoftDeleteSettings != nil && vault.Properties.SecuritySettings.SoftDeleteSettings.SoftDeleteState != nil {
			isSoftDeleteEnabled = *vault.Properties.SecuritySettings.SoftDeleteSettings.SoftDeleteState != vaults.SoftDeleteStateDisabled
		}
		if vault.Properties != nil && vault.Properties.SecuritySettings != nil && vault.Properties.SecuritySettings.ImmutabilitySettings != nil && vault.Properties.SecuritySettings.ImmutabilitySettings.State != nil {
			isImmutable = *vault.Properties.SecuritySettings.ImmutabilitySettings.State != vaults.ImmutabilityStateDisabled
		}

		if vault.Properties != nil && vault.Properties.SecuritySettings != nil && vault.Properties.SecuritySettings.MultiUserAuthorization != nil {
			isMUA = *vault.Properties.SecuritySettings.MultiUserAuthorization != vaults.MultiUserAuthorizationDisabled
		}

		if isSoftDeleteEnabled || isImmutable || isMUA {
			patch := vaults.PatchVault{
				Properties: &vaults.VaultProperties{
					SecuritySettings: &vaults.SecuritySettings{
						ImmutabilitySettings: &vaults.ImmutabilitySettings{
							State: pointer.To(vaults.ImmutabilityStateDisabled),
						},
						SoftDeleteSettings: &vaults.SoftDeleteSettings{
							SoftDeleteState: pointer.To(vaults.SoftDeleteStateDisabled),
						},
						MultiUserAuthorization: pointer.To(vaults.MultiUserAuthorizationDisabled),
					},
				},
			}

			if err := vaultsClient.UpdateThenPoll(ctx, *vaultId, patch, vaults.DefaultUpdateOperationOptions()); err != nil {
				log.Printf("updating %s to not be mutable: %+v", vaultId, err)
				continue
			}
		}

		backupItemsVaultId, err := backupprotecteditems.ParseVaultID(*vault.Id)
		if err != nil {
			log.Printf("[DEBUG] parsing id %q: %+v", *vault.Id, err)
			continue
		}

		backupItems, err := backupProtectedItemsClient.List(ctx, *backupItemsVaultId, backupprotecteditems.ListOperationOptions{})
		if err != nil || backupItems.Model == nil {
			log.Printf("listing Backup Protected Items for %q: %+v", backupItemsVaultId.ID(), err)
			continue
		}

		for _, backupItem := range *backupItems.Model {
			if backupItem.Id == nil {
				continue
			}

			backupItemId, err := protecteditems.ParseProtectedItemID(*backupItem.Id)
			if err != nil {
				log.Printf("[DEBUG] parsing id %q: %+v", *backupItemId, err)
				continue
			}

			// This process takes awhile and even after completing we don't have a guarantee that the vault can't see these items anymore so we'll just fire and forget
			// and expect this cleaner to have to run multiple times to get everything cleared out
			_, err = protectedItemsClient.Delete(ctx, *backupItemId)
			if err != nil {
				log.Printf("[DEBUG] deleting %q: %+v", backupItemId, err)
				continue
			}
		}

		// Azure doesn't return an error when the vault fails deleting when using DeleteThenPoll so we'll just fire and forget and expect this to have to run multiple times to get everything cleaned out
		if _, err := vaultsClient.Delete(ctx, *vaultId); err != nil {
			log.Printf("[DEBUG] deleting %q: %+v", vaultId.ID(), err)
			continue
		}
	}

	return nil
}
