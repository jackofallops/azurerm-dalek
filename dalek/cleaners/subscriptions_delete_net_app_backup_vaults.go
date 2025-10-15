package cleaners

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/go-azure-helpers/lang/pointer"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-11-01/backuppolicy"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-11-01/backups"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-11-01/backupvaults"
	"github.com/hashicorp/go-azure-sdk/sdk/client/pollers"
	"github.com/jackofallops/azurerm-dalek/clients"
	"github.com/jackofallops/azurerm-dalek/dalek/options"
)

func deleteBackupVaults(ctx context.Context, accountIdForBackupVault backupvaults.NetAppAccountId, client *clients.AzureClient, opts options.Options) error {
	netAppBackupVaultsClient := client.ResourceManager.NetAppBackupVaultsClient
	backupVaultsList, err := netAppBackupVaultsClient.ListByNetAppAccountComplete(ctx, accountIdForBackupVault)
	if err != nil {
		return fmt.Errorf("listing NetApp Backup Vaults for %s: %+v", accountIdForBackupVault, err)
	}

	log.Printf("[DEBUG] Found %d NetApp Backup Vaults in %s", len(backupVaultsList.Items), accountIdForBackupVault.NetAppAccountName)
	for _, vault := range backupVaultsList.Items {
		if vault.Id == nil {
			continue
		}

		accountIdForBackupPolicy, err := backuppolicy.ParseNetAppAccountID(accountIdForBackupVault.ID())
		if err != nil {
			return fmt.Errorf("parsing NetApp Account ID for Backup Policies: %+v", err)
		}
		if err := deleteBackupPolicies(ctx, pointer.From(accountIdForBackupPolicy), client, opts); err != nil {
			return err
		}
		vaultIdForBackups, err := backups.ParseBackupVaultID(pointer.From(vault.Id))
		if err != nil {
			return err
		}
		if err := deleteBackups(ctx, pointer.From(vaultIdForBackups), client, opts); err != nil {
			return err
		}

		vaultIdForVault, err := backupvaults.ParseBackupVaultID(*vault.Id)
		if err != nil {
			return err
		}
		if !opts.ActuallyDelete {
			log.Printf("[DEBUG] Would have deleted %s", vaultIdForVault)
			continue
		}
		response, err := netAppBackupVaultsClient.Delete(ctx, *vaultIdForVault)
		if err != nil {
			return err
		}

		pollerType, err := NewLROPoller(&lroClientAdapter{inner: netAppBackupVaultsClient.Client}, response.HttpResponse)
		if err != nil {
			return fmt.Errorf("creating poller for %s: %+v", vaultIdForVault, err)
		}
		if pollerType != nil {
			poller := pollers.NewPoller(pollerType, 0, 10)
			if err := poller.PollUntilDone(ctx); err != nil {
				return fmt.Errorf("polling delete operation for %s: %+v", vaultIdForVault, err)
			}
		}
		vaultsList, err := netAppBackupVaultsClient.ListByNetAppAccountComplete(ctx, accountIdForBackupVault)
		if err != nil {
			return fmt.Errorf("listing NetApp Backup Vaults after deletion for %s: %+v", accountIdForBackupVault, err)
		}
		for _, v := range vaultsList.Items {
			if v.Id != nil && *v.Id == vaultIdForVault.String() {
				return fmt.Errorf("[ERROR] Backup vault %s still exists after delete attempt", vaultIdForVault.String())
			}
		}
		log.Printf("[DEBUG] Deleted %s", vaultIdForVault)
	}
	return nil
}
