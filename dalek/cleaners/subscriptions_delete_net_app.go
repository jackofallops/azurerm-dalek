package cleaners

import (
	"context"
	"fmt"
	"github.com/fatih/color"
	"github.com/hashicorp/go-azure-helpers/lang/pointer"
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
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2025-01-01/snapshots"
	"github.com/jackofallops/azurerm-dalek/clients"
	"github.com/jackofallops/azurerm-dalek/dalek/options"
)

// TODO when there is a way to check the long-running operation status URL on delete operations (not possible in current
// version of SDK), the resource-still-exists error messages will be able to include why deletes fail, or even better,
// can inform how to modify the Dalek to preemptively eliminate deletion blockers.

type deleteNetAppSubscriptionCleaner struct{}

var _ SubscriptionCleaner = deleteNetAppSubscriptionCleaner{}
var waitSecondsAfterDeletion = 60 * time.Second

func (p deleteNetAppSubscriptionCleaner) Name() string {
	return "Removing Net App"
}

func (p deleteNetAppSubscriptionCleaner) Cleanup(ctx context.Context, subscriptionId commonids.SubscriptionId, client *clients.AzureClient, opts options.Options) error {
	if accountLists, err := client.ResourceManager.NetAppAccountClient.AccountsListBySubscription(ctx, subscriptionId); err != nil {
		return fmt.Errorf("listing NetApp Accounts for %s: %+v", subscriptionId, err)
	} else if accountLists.Model == nil {
		return fmt.Errorf("listing NetApp Accounts: model was nil")
	} else {
		log.Printf("[DEBUG] Found %d NetApp Accounts", len(*accountLists.Model))
		for _, account := range *accountLists.Model {
			if err := deepDeleteNetAppAccount(ctx, pointer.From(account.Id), subscriptionId, client, opts); err != nil {
				log.Printf("deleting NetApp Account %s: %+v", pointer.From(account.Id), err)
			}
		}
	}
	return nil
}

func deepDeleteNetAppAccount(ctx context.Context, id string, subscriptionId commonids.SubscriptionId, client *clients.AzureClient, opts options.Options) error {
	if id == "" {
		return nil
	}
	netAppAccountClient := client.ResourceManager.NetAppAccountClient
	accountId, err := netappaccounts.ParseNetAppAccountID(id)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(strings.ToLower(accountId.ResourceGroupName), strings.ToLower(opts.Prefix)) {
		log.Printf("[DEBUG] Not deleting %q as it does not match target RG prefix %q", accountId.ResourceGroupName, opts.Prefix)
		return nil
	}

	if err := deepDeleteBackupVaults(ctx, id, client, opts); err != nil {
		return err
	}

	if err := deepDeleteCapacityPools(ctx, id, client, opts); err != nil {
		return err
	}

	if !opts.ActuallyDelete {
		log.Printf("[DEBUG] Would have deleted %s", accountId)
	} else {
		if _, err := netAppAccountClient.AccountsDelete(ctx, *accountId); err != nil {
			return err
		}
		time.Sleep(waitSecondsAfterDeletion)
		acctList, err := netAppAccountClient.AccountsListBySubscription(ctx, subscriptionId)
		if err == nil && acctList.Model != nil {
			for _, acct := range *acctList.Model {
				if acct.Id != nil && *acct.Id == accountId.String() {
					return fmt.Errorf("[ERROR] NetApp account %s still exists after delete attempt.", accountId.String())
				}
			}
		}
		log.Printf(color.HiGreenString("[DEBUG] Deleted %s", accountId))
	}
	return nil
}

func deepDeleteCapacityPools(ctx context.Context, accountId string, client *clients.AzureClient, opts options.Options) error {
	netAppCapacityPoolClient := client.ResourceManager.NetAppCapacityPoolClient
	accountIdForCapacityPool, _ := capacitypools.ParseNetAppAccountID(accountId)
	capacityPoolList, err := netAppCapacityPoolClient.PoolsListComplete(ctx, *accountIdForCapacityPool)
	if err != nil {
		return fmt.Errorf("listing NetApp Capacity Pools for %s: %+v", accountIdForCapacityPool)
	}

	log.Printf("Found %d NetApp Capacity Pools", len(capacityPoolList.Items))
	for _, capacityPool := range capacityPoolList.Items {
		if capacityPool.Id == nil {
			continue
		}

		if err := deepDeleteVolumes(ctx, *capacityPool.Id, client, opts); err != nil {
			return err
		}

		capacityPoolId, err := capacitypools.ParseCapacityPoolID(*capacityPool.Id)
		if err != nil {
			return err
		}

		if !opts.ActuallyDelete {
			log.Printf("[DEBUG] Would have deleted %s", capacityPoolId)
		} else {
			if _, err := netAppCapacityPoolClient.PoolsDelete(ctx, *capacityPoolId); err != nil {
				return err
			}
			time.Sleep(waitSecondsAfterDeletion)
			poolList, err := netAppCapacityPoolClient.PoolsListComplete(ctx, *accountIdForCapacityPool)
			if err == nil {
				for _, pool := range poolList.Items {
					if pool.Id != nil && *pool.Id == capacityPoolId.String() {
						return fmt.Errorf("[ERROR] Capacity pool %s still exists after delete attempt.", capacityPoolId.String())
					}
				}
			}
			log.Printf("[DEBUG] Deleted %s", capacityPoolId)
		}
	}
	return nil
}

func deepDeleteVolumes(ctx context.Context, poolId string, client *clients.AzureClient, opts options.Options) error {
	capacityPoolForVolumesId, err := volumes.ParseCapacityPoolID(poolId)
	if err != nil {
		return err
	}

	netAppVolumeClient := client.ResourceManager.NetAppVolumeClient
	volumeList, err := netAppVolumeClient.ListComplete(ctx, *capacityPoolForVolumesId)
	if err != nil {
		return fmt.Errorf("listing NetApp Volumes for %s: %+v", capacityPoolForVolumesId, err)
	}

	log.Printf("Found %d NetApp Volumes", len(volumeList.Items))
	for _, volume := range volumeList.Items {
		if volume.Id == nil {
			continue
		}

		volumeId, err := volumes.ParseVolumeID(*volume.Id)
		if err != nil {
			return err
		}

		if err := deleteSnapshots(ctx, *volume.Id, client, opts); err != nil {
			return err
		}

		volumeIdForReplication, err := volumesreplication.ParseVolumeID(*volume.Id)
		if err != nil {
			return err
		}

		if !opts.ActuallyDelete {
			log.Printf("[DEBUG] Would have deleted %s", volumeIdForReplication)
		} else {
			netAppVolumeReplicationClient := client.ResourceManager.NetAppVolumeReplicationClient
			if _, err := netAppVolumeReplicationClient.VolumesDeleteReplication(ctx, *volumeIdForReplication); err != nil {
				return err
			}
			log.Printf("[DEBUG] Deleted replication for %s", volumeIdForReplication)
		}

		if !opts.ActuallyDelete {
			log.Printf("[DEBUG] Would have deleted %s", volumeId)
		} else {
			forceDelete := true
			if _, err := netAppVolumeClient.Delete(ctx, *volumeId, volumes.DeleteOperationOptions{ForceDelete: &forceDelete}); err != nil {
				return err
			}
			time.Sleep(waitSecondsAfterDeletion)
			vol, err := netAppVolumeClient.Get(ctx, *volumeId)
			if err == nil && vol.Model != nil {
				return fmt.Errorf("[ERROR] %s still exists after delete attempt.", volumeId)
			}
			log.Printf("[DEBUG] Deleted %s", volumeId)
		}
	}
	return nil
}

func deepDeleteBackupVaults(ctx context.Context, id string, client *clients.AzureClient, opts options.Options) error {
	accountIdForBackupVault, err := backupvaults.ParseNetAppAccountID(id)
	if err != nil {
		return fmt.Errorf("[ERROR] Unable to parse NetApp Account ID for Backup Vaults: %+v", err)
	}

	netAppBackupVaultsClient := client.ResourceManager.NetAppBackupVaultsClient
	backupVaultsList, err := netAppBackupVaultsClient.ListByNetAppAccountComplete(ctx, *accountIdForBackupVault)
	if err != nil {
		return fmt.Errorf("listing NetApp Backup Vaults for %s: %+v", accountIdForBackupVault, err)
	}

	for _, vault := range backupVaultsList.Items {
		if vault.Id == nil {
			continue
		}

		if err := deleteBackupPolicies(ctx, *vault.Id, client, opts); err != nil {
			return err
		}

		if err := deleteBackups(ctx, *vault.Id, client, opts); err != nil {
			return err
		}

		if !opts.ActuallyDelete {
			log.Printf("[DEBUG] Would have deleted %s", *vault.Id)
		} else {
			vaultIdForBackup, err := backupvaults.ParseBackupVaultID(*vault.Id)
			if err != nil {
				return err
			}
			if _, err := netAppBackupVaultsClient.Delete(ctx, *vaultIdForBackup); err != nil {
				return err
			}
			time.Sleep(waitSecondsAfterDeletion)
			vaultsList, err := netAppBackupVaultsClient.ListByNetAppAccountComplete(ctx, *accountIdForBackupVault)
			if err != nil {
				return fmt.Errorf("listing NetApp Backup Vaults after deletion for %s: %+v", accountIdForBackupVault, err)
			} else {
				for _, v := range vaultsList.Items {
					if v.Id != nil && *v.Id == vaultIdForBackup.String() {
						return fmt.Errorf("[ERROR] Backup vault %s still exists after delete attempt.", vaultIdForBackup.String())
					}
				}
			}
			log.Printf("[DEBUG] Deleted %s", vaultIdForBackup)
		}
	}
	return nil
}

func deleteBackupPolicies(ctx context.Context, vaultId string, client *clients.AzureClient, opts options.Options) error {
	return nil
}

func deleteSnapshots(ctx context.Context, volumeId string, client *clients.AzureClient, opts options.Options) error {
	volumeIdForSnapshots, err := snapshots.ParseVolumeID(volumeId)
	if err != nil {
		return err
	}

	snapshotClient := client.ResourceManager.NetAppSnapshotClient
	resp, err := snapshotClient.List(ctx, *volumeIdForSnapshots)
	if err != nil {
		return err
	}
	if resp.Model == nil {
		return fmt.Errorf("listing NetApp Snapshots for %s: model was nil", volumeIdForSnapshots)
	}
	if resp.Model.Value == nil {
		return fmt.Errorf("listing NetApp Snapshots for %s: value was nil", volumeIdForSnapshots)
	}
	log.Printf("Found %d NetApp Snapshots", len(*resp.Model.Value))
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
		} else {
			if _, err := snapshotClient.Delete(ctx, *snapshotID); err != nil {
				return err
			} else {
				time.Sleep(waitSecondsAfterDeletion)
				if b, err := snapshotClient.Get(ctx, *snapshotID); err != nil {
					return err
				} else if b.Model != nil {
					return fmt.Errorf("[ERROR] %s still exists after delete attempt. The long-running operation status URL might have more information", snapshotID.String())
				}
				log.Printf("[DEBUG] Deleted %s", snapshotID)
			}
		}
	}
	return nil
}

func deleteBackups(ctx context.Context, vaultId string, client *clients.AzureClient, opts options.Options) error {
	backupsVaultId, err := backups.ParseBackupVaultID(vaultId)
	if err != nil {
		return err
	}

	netAppBackupsClient := client.ResourceManager.NetAppBackupsClient
	backupsList, err := netAppBackupsClient.ListByVaultComplete(ctx, *backupsVaultId, backups.ListByVaultOperationOptions{})
	if err != nil {
		return err
	}
	log.Printf("Found %d NetApp Backups", len(backupsList.Items))
	for _, backup := range backupsList.Items {
		if backup.Id == nil {
			continue
		}
		backupId, err := backups.ParseBackupID(*backup.Id)
		if err != nil {
			return err
		}
		if !opts.ActuallyDelete {
			log.Printf("[DEBUG] Would have deleted %s", backupId.String())
			continue
		} else {
			if _, err := netAppBackupsClient.Delete(ctx, *backupId); err != nil {
				return err
			}
			time.Sleep(waitSecondsAfterDeletion)
			b, err := netAppBackupsClient.Get(ctx, *backupId)
			if err == nil && b.Model != nil {
				return fmt.Errorf("[ERROR] %s still exists after delete attempt.", backupId.String())
			}
			log.Printf("[DEBUG] Deleted %s", backupId)
		}
	}
	return nil
}
