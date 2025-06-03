package cleaners

import (
	"context"
	"fmt"
	"github.com/fatih/color"
	"log"
	"strings"
	"time"

	"github.com/hashicorp/go-azure-helpers/resourcemanager/commonids"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-05-01/capacitypools"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-05-01/netappaccounts"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-05-01/volumes"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-05-01/volumesreplication"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2025-01-01/backups"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2025-01-01/backupvaults"
	"github.com/jackofallops/azurerm-dalek/clients"
	"github.com/jackofallops/azurerm-dalek/dalek/options"
)

type deleteNetAppSubscriptionCleaner struct{}

var _ SubscriptionCleaner = deleteNetAppSubscriptionCleaner{}

func (p deleteNetAppSubscriptionCleaner) Name() string {
	return "Removing Net App"
}

/*
Nesting of Net App resources is such that deletion of Net App accounts must happen in the following order:

1. Volume Replications
2. Volumes
3. Capacity Pools
4. Backups
5. Backup Vaults
6. Net App Accounts
*/

func (p deleteNetAppSubscriptionCleaner) Cleanup(ctx context.Context, subscriptionId commonids.SubscriptionId, client *clients.AzureClient, opts options.Options) error {
	netAppAccountClient := client.ResourceManager.NetAppAccountClient
	netAppCapacityPoolClient := client.ResourceManager.NetAppCapacityPoolClient
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

	log.Printf("[DEBUG] Found %d NetApp Accounts", len(*accountLists.Model))

	waitSecondsAfterDeletion := 60 * time.Second

	for _, account := range *accountLists.Model {
		if account.Id == nil {
			continue
		}
		nestedResourceFailed := false

		accountIdForCapacityPool, err := capacitypools.ParseNetAppAccountID(*account.Id)
		if err != nil {
			return err
		}

		if !strings.HasPrefix(strings.ToLower(accountIdForCapacityPool.ResourceGroupName), strings.ToLower(opts.Prefix)) {
			log.Printf("[DEBUG]  Resource Group \"%s\" shouldn't be deleted - Skipping..", accountIdForCapacityPool.ResourceGroupName)
			continue
		}

		capacityPoolList, err := netAppCapacityPoolClient.PoolsListComplete(ctx, *accountIdForCapacityPool)
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
					log.Printf("[ERROR] Failed to parse volume ID: %+v", err)
					nestedResourceFailed = true
					break
				}

				volumeIdForReplication, err := volumesreplication.ParseVolumeID(*volume.Id)
				if err != nil {
					log.Printf("[ERROR] Failed to parse volume replication ID: %+v", err)
					nestedResourceFailed = true
					break
				}

				if !opts.ActuallyDelete {
					log.Printf("[DEBUG] Would have deleted %s", volumeIdForReplication)
				} else {
					if _, err := netAppVolumeReplicationClient.VolumesDeleteReplication(ctx, *volumeIdForReplication); err != nil {
						log.Printf(color.RedString("[ERROR] Failed to delete volume replication %s: %+v", volumeIdForReplication, err))
						nestedResourceFailed = true
						break
					}
					log.Printf(color.GreenString("[DEBUG] Deleted replication for %s", volumeIdForReplication))
				}

				if !opts.ActuallyDelete {
					log.Printf("[DEBUG] Would have deleted %s", volumeId)
				} else {
					forceDelete := true
					if _, err := netAppVolumeClient.Delete(ctx, *volumeId, volumes.DeleteOperationOptions{ForceDelete: &forceDelete}); err != nil {
						log.Printf(color.RedString("[ERROR] Failed to delete volume %s: %+v", volumeId, err))
						nestedResourceFailed = true
						break
					}
					time.Sleep(waitSecondsAfterDeletion)
					vol, err := netAppVolumeClient.Get(ctx, *volumeId)
					if err == nil && vol.Model != nil {
						log.Printf(color.RedString("[ERROR] %s still exists after delete attempt.", volumeId))
						nestedResourceFailed = true
						break
					}
					log.Printf(color.GreenString("[DEBUG] Deleted %s", volumeId))
				}
			}

			if nestedResourceFailed {
				break
			}

			capacityPoolId, err := capacitypools.ParseCapacityPoolID(*capacityPool.Id)
			if err != nil {
				return err
			}

			if !opts.ActuallyDelete {
				log.Printf("[DEBUG] Would have deleted %s", capacityPoolId)
			} else {
				if _, err := netAppCapacityPoolClient.PoolsDelete(ctx, *capacityPoolId); err != nil {
					log.Printf(color.RedString("[ERROR] Failed to delete capacity pool %s: %+v", capacityPoolId, err))
					nestedResourceFailed = true
					break
				}
				time.Sleep(waitSecondsAfterDeletion)
				poolList, err := netAppCapacityPoolClient.PoolsListComplete(ctx, *accountIdForCapacityPool)
				if err == nil {
					for _, pool := range poolList.Items {
						if pool.Id != nil && *pool.Id == capacityPoolId.String() {
							log.Printf(color.RedString("[ERROR] Capacity pool %s still exists after delete attempt.", capacityPoolId.String()))
							nestedResourceFailed = true
							break
						}
					}
				}
				log.Printf(color.GreenString("[DEBUG] Deleted %s", capacityPoolId))
			}

			if nestedResourceFailed {
				break
			}
		}

		if nestedResourceFailed {
			log.Printf("[ERROR] Skipping NetApp account %s due to nested resource deletion failure.", *account.Id)
			continue
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

			vaultIdForBackup, err := backupvaults.ParseBackupVaultID(*vault.Id)
			if err != nil {
				log.Printf("[ERROR] Couldn't parse vault ID: %+v", err)
				nestedResourceFailed = true
				break
			}

			backupsVaultId, err := backups.ParseBackupVaultID(*vault.Id)
			if err != nil {
				log.Printf("[ERROR] Couldn't convert vault ID for backups: %+v", err)
				nestedResourceFailed = true
				break
			}
			backupsList, err := netAppBackupsClient.ListByVaultComplete(ctx, *backupsVaultId, backups.ListByVaultOperationOptions{})
			if err != nil {
				log.Printf("[ERROR] Couldn't list backups for vault %s: %+v", vaultIdForBackup, err)
				nestedResourceFailed = true
				break
			}
			for _, backup := range backupsList.Items {
				if backup.Id == nil {
					continue
				}
				backupId, err := backups.ParseBackupID(*backup.Id)
				if err != nil {
					log.Printf("[ERROR] Couldn't parse backup ID: %+v", err)
					nestedResourceFailed = true
					break
				}
				if !opts.ActuallyDelete {
					log.Printf("[DEBUG] Would have deleted %s", backupId.String())
					continue
				} else {
					if _, err := netAppBackupsClient.Delete(ctx, *backupId); err != nil {
						log.Printf(color.RedString("[ERROR] Failed to delete backup %s: %+v", backupId.String(), err))
						nestedResourceFailed = true
						break
					}
					time.Sleep(waitSecondsAfterDeletion)
					b, err := netAppBackupsClient.Get(ctx, *backupId)
					if err == nil && b.Model != nil {
						log.Printf(color.RedString("[ERROR] %s still exists after delete attempt.", backupId.String()))
						nestedResourceFailed = true
						break
					}
					log.Printf(color.GreenString("[DEBUG] Deleted %s", backupId))
				}
			}
			if nestedResourceFailed {
				break
			}

			if !opts.ActuallyDelete {
				log.Printf("[DEBUG] Would have deleted %s", vaultIdForBackup.String())
			} else {
				if _, err := netAppBackupVaultsClient.Delete(ctx, *vaultIdForBackup); err != nil {
					log.Printf(color.RedString("[ERROR] Failed to delete backup vault %s: %+v", vaultIdForBackup.String(), err))
					nestedResourceFailed = true
					break
				}
				time.Sleep(waitSecondsAfterDeletion)
				vaultsList, err := netAppBackupVaultsClient.ListByNetAppAccountComplete(ctx, *accountIdForBackupVault)
				if err == nil {
					for _, v := range vaultsList.Items {
						if v.Id != nil && *v.Id == vaultIdForBackup.String() {
							log.Printf(color.RedString("[ERROR] Backup vault %s still exists after delete attempt.", vaultIdForBackup.String()))
							nestedResourceFailed = true
							break
						}
					}
				}
				log.Printf(color.GreenString("[DEBUG] Deleted %s", vaultIdForBackup))
			}
		}
		if nestedResourceFailed {
			log.Printf("[ERROR] Skipping NetApp account %s due to nested resource deletion failure.", *account.Id)
			continue
		}

		accountId, err := netappaccounts.ParseNetAppAccountID(*account.Id)
		if err != nil {
			return err
		}

		if !opts.ActuallyDelete {
			log.Printf("[DEBUG] Would have deleted %s", accountId)
		} else {
			if _, err := netAppAccountClient.AccountsDelete(ctx, *accountId); err != nil {
				log.Printf("[ERROR] Failed to delete NetApp account %s: %+v", accountId, err)
				continue
			}
			time.Sleep(waitSecondsAfterDeletion)
			acctList, err := netAppAccountClient.AccountsListBySubscription(ctx, subscriptionId)
			if err == nil && acctList.Model != nil {
				for _, acct := range *acctList.Model {
					if acct.Id != nil && *acct.Id == accountId.String() {
						log.Printf("[ERROR] NetApp account %s still exists after delete attempt.", accountId.String())
						break
					}
				}
			}
			log.Printf(color.HiGreenString("[DEBUG] Deleted %s", accountId))
		}
	}
	return nil
}
