package cleaners

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/hashicorp/go-azure-helpers/lang/pointer"
	"github.com/hashicorp/go-azure-helpers/resourcemanager/commonids"
	"github.com/hashicorp/go-azure-sdk/resource-manager/dataprotection/2024-04-01/backupinstances"
	"github.com/hashicorp/go-azure-sdk/resource-manager/dataprotection/2024-04-01/backuppolicies"
	"github.com/hashicorp/go-azure-sdk/resource-manager/dataprotection/2024-04-01/backupvaults"
	"github.com/hashicorp/go-azure-sdk/resource-manager/dataprotection/2024-04-01/deletedbackupinstances"
	"github.com/jackofallops/azurerm-dalek/clients"
	"github.com/jackofallops/azurerm-dalek/dalek/options"
)

var _ ResourceGroupCleaner = removeDataProtectionFromResourceGroupCleaner{}

type removeDataProtectionFromResourceGroupCleaner struct{}

func (removeDataProtectionFromResourceGroupCleaner) Name() string {
	return "Removing Data Protection"
}

func (removeDataProtectionFromResourceGroupCleaner) Cleanup(ctx context.Context, id commonids.ResourceGroupId, client *clients.AzureClient, opts options.Options) error {
	backupVaults, err := client.ResourceManager.DataProtection.BackupVaults.GetInResourceGroupComplete(ctx, id)
	if err != nil {
		log.Printf("[DEBUG] Error retrieving the Backup Vaults within %s: %+v", id, err)
	}
	errs := make([]error, 0)
	for _, vault := range backupVaults.Items {
		vaultId := backupvaults.NewBackupVaultID(id.SubscriptionId, id.ResourceGroupName, *vault.Name)

		// disable soft-delete first, this will block the deletion of the vault if instances are soft-deleted
		patch := backupvaults.PatchResourceRequestInput{
			Properties: &backupvaults.PatchBackupVaultInput{
				SecuritySettings: &backupvaults.SecuritySettings{
					SoftDeleteSettings: &backupvaults.SoftDeleteSettings{
						State: pointer.To(backupvaults.SoftDeleteStateOff),
					},
				},
			},
		}
		if err := client.ResourceManager.DataProtection.BackupVaults.UpdateThenPoll(ctx, vaultId, patch, backupvaults.DefaultUpdateOperationOptions()); err != nil {
			log.Printf("Failed to turn off Soft Delete for %s: %+v", vaultId, err)
			errs = append(errs, fmt.Errorf("turning off Soft Delete for %s: %+v", vaultId, err))
		}

		// We have to undelete items that were deleted when softdelete was enabled and then delete them again
		deletedBackupInstanceVaultId := deletedbackupinstances.NewBackupVaultID(vaultId.SubscriptionId, vaultId.ResourceGroupName, vaultId.BackupVaultName)
		deletedInstances, err := client.ResourceManager.DataProtection.DeletedBackupInstances.ListComplete(ctx, deletedBackupInstanceVaultId)
		if err != nil {
			log.Printf("deleted Backup Instances within %s was empty: %+v", deletedBackupInstanceVaultId, err)
		} else {
			for _, deletedInstance := range deletedInstances.Items {
				deletedInstanceId := deletedbackupinstances.NewDeletedBackupInstanceID(deletedBackupInstanceVaultId.SubscriptionId, deletedBackupInstanceVaultId.ResourceGroupName, deletedBackupInstanceVaultId.BackupVaultName, *deletedInstance.Name)

				if !opts.ActuallyDelete {
					log.Printf("[DEBUG] Would have deleted %s..", deletedInstanceId)
					continue
				}

				log.Printf("[DEBUG] Deleting %s..", deletedInstanceId)
				if err := client.ResourceManager.DataProtection.DeletedBackupInstances.UndeleteThenPoll(ctx, deletedInstanceId); err != nil {
					log.Printf("[ERROR] deleting %s: %+v", deletedInstanceId, err)
					errs = append(errs, fmt.Errorf("undeleting %s: %+v", deletedInstanceId, err))
				}
				log.Printf("[DEBUG] Deleted %s.", deletedInstanceId)
			}
		}

		// list the Backup Instances within it, those need to be removed first
		backupInstancesVaultId := backupinstances.NewBackupVaultID(vaultId.SubscriptionId, vaultId.ResourceGroupName, vaultId.BackupVaultName)
		instances, err := client.ResourceManager.DataProtection.BackupInstances.ListComplete(ctx, backupInstancesVaultId)
		if err != nil {
			errs = append(errs, fmt.Errorf("listing Backup Instances within %s: %+v", backupInstancesVaultId, err))
		} else {
			for _, instance := range instances.Items {
				instanceId := backupinstances.NewBackupInstanceID(backupInstancesVaultId.SubscriptionId, backupInstancesVaultId.ResourceGroupName, backupInstancesVaultId.BackupVaultName, *instance.Name)
				if !opts.ActuallyDelete {
					log.Printf("[DEBUG] Would have deleted %s..", instanceId)
					continue
				}

				log.Printf("[DEBUG] Deleting %s..", instanceId)
				if err := client.ResourceManager.DataProtection.BackupInstances.DeleteThenPoll(ctx, instanceId, backupinstances.DefaultDeleteOperationOptions()); err != nil {
					errs = append(errs, fmt.Errorf("deleting %s: %+v", instanceId, err))
					continue
				}
				log.Printf("[DEBUG] Deleted %s.", instanceId)
			}
		}

		// then let's go through and remove the Backup Policies
		backupPoliciesVaultId := backuppolicies.NewBackupVaultID(vaultId.SubscriptionId, vaultId.ResourceGroupName, vaultId.BackupVaultName)
		policies, err := client.ResourceManager.DataProtection.BackupPolicies.ListComplete(ctx, backupPoliciesVaultId)
		if err != nil {
			errs = append(errs, fmt.Errorf("listing Backup Policies within %s: %+v", backupPoliciesVaultId, err))
		} else {
			for _, policy := range policies.Items {
				policyId := backuppolicies.NewBackupPolicyID(backupPoliciesVaultId.SubscriptionId, backupPoliciesVaultId.ResourceGroupName, backupPoliciesVaultId.BackupVaultName, *policy.Name)
				if !opts.ActuallyDelete {
					log.Printf("[DEBUG] Would have deleted %s..", policyId)
					continue
				}

				log.Printf("[DEBUG] Deleting %s..", policyId)
				if _, err := client.ResourceManager.DataProtection.BackupPolicies.Delete(ctx, policyId); err != nil {
					errs = append(errs, fmt.Errorf("deleting %s: %+v", policyId, err))
					continue
				}
				log.Printf("[DEBUG] Deleted %s.", policyId)
			}
		}

		if !opts.ActuallyDelete {
			log.Printf("[DEBUG] Would have deleted %s..", vaultId)
			continue
		}
		log.Printf("[DEBUG] Deleting %s..", vaultId)
		if err := client.ResourceManager.DataProtection.BackupVaults.DeleteThenPoll(ctx, vaultId); err != nil {
			errs = append(errs, fmt.Errorf("deleting %s: %+v", vaultId, err))
			continue
		}
		log.Printf("[DEBUG] Deleted %s.", vaultId)
	}

	return errors.Join(errs...)
}

func (removeDataProtectionFromResourceGroupCleaner) ResourceTypes() []string {
	return []string{
		"Microsoft.DataProtection/backupVaults",
	}
}
