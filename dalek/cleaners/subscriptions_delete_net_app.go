package cleaners

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hashicorp/go-azure-helpers/lang/response"
	"github.com/hashicorp/go-azure-helpers/resourcemanager/commonids"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-05-01/capacitypools"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-05-01/netappaccounts"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-05-01/volumes"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-05-01/volumesreplication"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2025-01-01/backups"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2025-01-01/backupvaults"
	"github.com/jackofallops/azurerm-dalek/clients"
	"github.com/jackofallops/azurerm-dalek/dalek/options"
	"golang.org/x/sync/errgroup"
)

type deleteNetAppSubscriptionCleaner struct{}

var _ SubscriptionCleaner = deleteNetAppSubscriptionCleaner{}

func (p deleteNetAppSubscriptionCleaner) Name() string {
	return "Removing Net App"
}

func (p deleteNetAppSubscriptionCleaner) Cleanup(ctx context.Context, subscriptionId commonids.SubscriptionId, client *clients.AzureClient, opts options.Options) error {
	netAppAccountClient := client.ResourceManager.NetAppAccountClient
	netAppCapcityPoolClient := client.ResourceManager.NetAppCapacityPoolClient
	netAppVolumeClient := client.ResourceManager.NetAppVolumeClient
	netAppVolumeReplicationClient := client.ResourceManager.NetAppVolumeReplicationClient
	netAppBackupVaultsClient := client.ResourceManager.NetAppBackupVaultsClient
	netAppBackupsClient := client.ResourceManager.NetAppBackupsClient

	accountLists, err := netAppAccountClient.AccountsListBySubscription(ctx, subscriptionId)
	if err != nil {
		return fmt.Errorf("listing NetApp Accounts for %s: %+v", subscriptionId, err)
	}

	if accountLists.Model == nil {
		return fmt.Errorf("listing NetApp Accounts: model was nil")
	}

	for _, account := range *accountLists.Model {
		if account.Id == nil {
			continue
		}

		accountIdForCapacityPool, err := capacitypools.ParseNetAppAccountID(*account.Id)
		if err != nil {
			return err
		}

		if !strings.HasPrefix(accountIdForCapacityPool.ResourceGroupName, opts.Prefix) {
			log.Printf("[DEBUG] Not deleting %q as it does not match target RG prefix %q", *accountIdForCapacityPool, opts.Prefix)
			continue
		}

		capacityPoolList, err := netAppCapcityPoolClient.PoolsListComplete(ctx, *accountIdForCapacityPool)
		if err != nil {
			return fmt.Errorf("listing NetApp Capacity Pools for %s: %+v", accountIdForCapacityPool, err)
		}

		for _, capacityPool := range capacityPoolList.Items {
			if capacityPool.Id == nil {
				continue
			}

			capacityPoolForVolumesId, err := volumes.ParseCapacityPoolID(*capacityPool.Id)
			if err != nil {
				return err
			}

			volumeList, err := netAppVolumeClient.ListComplete(ctx, *capacityPoolForVolumesId)
			if err != nil {
				return fmt.Errorf("listing NetApp Volumes for %s: %+v", capacityPoolForVolumesId, err)
			}

			for _, volume := range volumeList.Items {
				if volume.Id == nil {
					continue
				}

				volumeId, err := volumes.ParseVolumeID(*volume.Id)
				if err != nil {
					return err
				}

				if !opts.ActuallyDelete {
					log.Printf("[DEBUG] Would have deleted %s..", volumeId)
					continue
				}

				volumeReplicationId, err := volumesreplication.ParseVolumeID(*volume.Id)
				if err != nil {
					return nil
				}

				if resp, err := netAppVolumeReplicationClient.VolumesDeleteReplication(ctx, *volumeReplicationId); err != nil {
					if !response.WasNotFound(resp.HttpResponse) {
						return fmt.Errorf("deleting replication for %s: %+v", volumeReplicationId, err)
					}
				}

				if opts.ActuallyDelete {
					// sleeping because there is some eventual consistency for when the replication decouples from the volume
					time.Sleep(30 * time.Second)
				}

				forceDelete := true

				if !opts.ActuallyDelete {
					log.Printf("[DEBUG] Would have deleted %s..", volumeId)
					continue
				}

				if result, err := netAppVolumeClient.Delete(ctx, *volumeId, volumes.DeleteOperationOptions{ForceDelete: &forceDelete}); err != nil {
					// Potential Eventual Consistency Issues so we'll just log and move on
					log.Printf("[DEBUG] Unable to delete %s: %+v", volumeId, err)
				} else {
					result.Poller.PollUntilDone(ctx)
				}
			}

			capacityPoolId, err := capacitypools.ParseCapacityPoolID(*capacityPool.Id)
			if err != nil {
				return err
			}

			// the netapp api doesn't error if the delete fails so we'll just fire and forget as to not break the dalek

			if !opts.ActuallyDelete {
				log.Printf("[DEBUG] Would have deleted %s..", capacityPoolId)
				continue
			}

			if result, err := netAppCapcityPoolClient.PoolsDelete(ctx, *capacityPoolId); err != nil {
				// Potential Eventual Consistency Issues so we'll just log and move on
				log.Printf("[DEBUG] Unable to delete %s: %+v", capacityPoolId, err)
			} else {
				result.Poller.PollUntilDone(ctx)
			}

			// sleeping because there is some eventual consistency for when the capacity pool decouples from the account
			time.Sleep(30 * time.Second)
		}

		accountIdForBackupVault, err := backupvaults.ParseNetAppAccountID(*account.Id)
		if err != nil {
			log.Printf("[DEBUG] Unable to parse NetApp Account ID for Backup Vaults: %+v", err)
			continue
		}
		backupVaultsList, err := netAppBackupVaultsClient.ListByNetAppAccountComplete(ctx, *accountIdForBackupVault)
		if err != nil {
			return fmt.Errorf("listing NetApp Backup Vaults for %s: %+v", accountIdForBackupVault, err)
		}

		for _, vault := range backupVaultsList.Items {
			if vault.Id == nil {
				continue
			}

			vaultIdForBackup, err := backups.ParseBackupVaultID(*vault.Id)
			if err != nil {
				log.Printf("[ERROR] Couldn't parse vault ID %s", *vault.Id)
			}
			backupsList, err := netAppBackupsClient.ListByVaultComplete(ctx, *vaultIdForBackup, backups.ListByVaultOperationOptions{})
			if err != nil {
				return fmt.Errorf("listing NetApp Backups for %s: %+v", vaultIdForBackup, err)
			}
			g, egctx := errgroup.WithContext(ctx) // re-use the caller’s ctx for cancellation
			for _, b := range backupsList.Items {
				backup := b // capture loop variable
				if backup.Id == nil {
					continue
				}
				backupIDPtr, err := backups.ParseBackupID(*backup.Id)
				if err != nil {
					return err
				}
				// Dereference once so each goroutine gets its own value copy.
				backupID := *backupIDPtr

				if !opts.ActuallyDelete {
					log.Printf("[DEBUG] Would have deleted %s..", backupID)
					continue
				}

				// Start all DeleteThenPolls in parallel, each in its own Go routine
				g.Go(func() error {
					if err := netAppBackupsClient.DeleteThenPoll(egctx, backupID); err != nil {
						log.Printf("[DEBUG] Unable to delete %s: %+v", backupID, err)
						return err // bubbles up to g.Wait()
					}
					return nil
				})
			}
			// Wait blocks until every g.Go() has finished. It returns the first non-nil error reported (if any).
			if err := g.Wait(); err != nil {
				return err
			}
			backupVaultId, err := backupvaults.ParseBackupVaultID(*vault.Id)

			if !opts.ActuallyDelete {
				log.Printf("[DEBUG] Would have deleted %s..", backupVaultId)
				continue
			}

			if result, err := netAppBackupVaultsClient.Delete(ctx, *backupVaultId); err != nil {
				log.Printf("[DEBUG] Unable to delete %s: %+v", backupVaultId, err)
			} else {
				if err := result.Poller.PollUntilDone(ctx); err != nil {
					log.Printf("[DEBUG] Unable to poll deletion status of %s: %+v", backupVaultId, err)
				}
			}
		}

		if opts.ActuallyDelete {
			maxAttempts := 4
			for attempt := range maxAttempts {
				vaults, _ := netAppBackupVaultsClient.ListByNetAppAccountComplete(ctx, *accountIdForBackupVault)
				if vaults.Items == nil || len(vaults.Items) == 0 {
					log.Printf("[DEBUG] Backup vaults successfully disassociated from NetApp account %s. Waiting 10 seconds just to allow eventual consistency to catch up.", accountIdForBackupVault)
					time.Sleep(10 * time.Second)
					break
				} else {
					log.Printf("[DEBUG] Attempt %d, Backup vaults not yet disassociated from NetApp account %s, waiting 30 seconds...", attempt+1, accountIdForBackupVault)
					time.Sleep(30 * time.Second)
				}
				if attempt == maxAttempts-1 {
					log.Printf("[DEBUG] Max retries reached, failed to disassociate Backup Vautls from NetApp account %s", accountIdForBackupVault)
				}
			}
		}

		accountId, err := netappaccounts.ParseNetAppAccountID(*account.Id)
		if err != nil {
			return err
		}

		if !opts.ActuallyDelete {
			log.Printf("[DEBUG] Would have deleted %s..", accountId)
			continue
		}
		// the netapp api doesn't error if the delete fails so we'll just fire and forget as to not break the dalek
		maxAttempts := 4
		for attempt := range maxAttempts {
			if result, err := netAppAccountClient.AccountsDelete(ctx, *accountId); err != nil {
				if !strings.Contains(err.Error(), "Cannot delete resource while nested resources exist") {
					log.Printf("[DEBUG] Unable to delete %s: %+v", accountId, err)
					break
				}
				log.Printf("[DEBUG] Attempt %d of %d, unable to delete NetApp account because it says it still has nested resources even though we deleted them. Waiting 30 seconds before retrying... %s: %+v", attempt+1, maxAttempts, accountId, err)
				time.Sleep(30 * time.Second)
			} else {
				result.Poller.PollUntilDone(ctx)
			}
		}
	}

	return nil
}
