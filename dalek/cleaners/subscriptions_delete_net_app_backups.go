package cleaners

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/go-azure-helpers/lang/pointer"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-11-01/backuppolicy"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-11-01/backups"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-11-01/volumes"
	"github.com/hashicorp/go-azure-sdk/sdk/client/pollers"
	"github.com/jackofallops/azurerm-dalek/clients"
	"github.com/jackofallops/azurerm-dalek/dalek/options"
)

func deleteBackups(ctx context.Context, backupsVaultId backups.BackupVaultId, client *clients.AzureClient, opts options.Options) error {
	netAppBackupsClient := client.ResourceManager.NetAppBackupsClient
	backupsList, err := netAppBackupsClient.ListByVaultComplete(ctx, backupsVaultId, backups.ListByVaultOperationOptions{})
	if err != nil {
		return err
	}
	log.Printf("[DEBUG] Found %d NetApp Backups in %s", len(backupsList.Items), backupsVaultId.NetAppAccountName)
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
		}
		response, err := netAppBackupsClient.Delete(ctx, *backupId)
		if err != nil {
			return err
		}
		pollerType, err := NewLROPoller(&lroClientAdapter{inner: netAppBackupsClient.Client}, response.HttpResponse)
		if err != nil {
			return fmt.Errorf("creating poller for %s: %+v", backupId, err)
		}
		if pollerType != nil {
			poller := pollers.NewPoller(pollerType, 0, 10)
			if err := poller.PollUntilDone(ctx); err != nil {
				return fmt.Errorf("polling delete operation for %s: %+v", backupId, err)
			}
		}
		b, err := netAppBackupsClient.Get(ctx, *backupId)
		if err == nil && b.Model != nil {
			return fmt.Errorf("[ERROR] %s still exists after delete attempt", backupId.String())
		}
		log.Printf("[DEBUG] Deleted %s", backupId)
	}
	return nil
}

func deleteBackupPolicies(ctx context.Context, accountIdForBackupPolicy backuppolicy.NetAppAccountId, client *clients.AzureClient, opts options.Options) error {
	backupsPolicyClient := client.ResourceManager.NetAppBackupPolicyClient
	backupPoliciesList, err := backupsPolicyClient.BackupPoliciesList(ctx, accountIdForBackupPolicy)
	if err != nil {
		return fmt.Errorf("listing NetApp Backup Policies for %s: %+v", accountIdForBackupPolicy, err)
	}

	log.Printf("[DEBUG] Found %d NetApp Backup Policies in %s", len(*backupPoliciesList.Model.Value), accountIdForBackupPolicy.NetAppAccountName)
	for _, policy := range *backupPoliciesList.Model.Value {
		if policy.Id == nil {
			continue
		}
		policyId, err := backuppolicy.ParseBackupPolicyID(*policy.Id)
		if err != nil {
			return fmt.Errorf("parsing Backup Policy ID %s: %+v", *policy.Id, err)
		}
		if !opts.ActuallyDelete {
			log.Printf("[DEBUG] Would have deleted %s", policyId)
			continue
		}
		if pointer.From(policy.Properties.VolumesAssigned) > 0 {
			log.Printf("[DEBUG] Detaching %d volumes from Backup Policy %s", pointer.From(policy.Properties.VolumesAssigned), policyId)
			volumesClient := client.ResourceManager.NetAppVolumeClient
			for _, volume := range *policy.Properties.VolumeBackups {
				log.Printf("[DEBUG] Detaching volume %s from Backup Policy %s", *volume.VolumeResourceId, policyId)
				volumeId, err := volumes.ParseVolumeID(*volume.VolumeResourceId)
				if err != nil {
					return fmt.Errorf("parsing Volume ID %s: %+v", *volume.VolumeResourceId, err)
				}
				err = volumesClient.UpdateThenPoll(ctx, *volumeId, volumes.VolumePatch{
					Properties: &volumes.VolumePatchProperties{
						DataProtection: &volumes.VolumePatchPropertiesDataProtection{
							Backup: &volumes.VolumeBackupProperties{
								BackupPolicyId: pointer.To(""),
							},
						},
					},
				})
				if err != nil {
					log.Printf("[ERROR] failed to detach volume")
					continue
				}
				log.Printf("[DEBUG] Detached volume %s from Backup Policy %s", *volume.VolumeResourceId, policyId)
			}
		}

		response, err := backupsPolicyClient.BackupPoliciesDelete(ctx, *policyId)
		if err != nil {
			return err
		}
		pollerType, err := NewLROPoller(&lroClientAdapter{inner: backupsPolicyClient.Client}, response.HttpResponse)
		if err != nil {
			return fmt.Errorf("creating poller for %s: %+v", policyId, err)
		}
		if pollerType != nil {
			poller := pollers.NewPoller(pollerType, 0, 10)
			if err := poller.PollUntilDone(ctx); err != nil {
				return fmt.Errorf("polling delete operation for %s: %+v", policyId, err)
			}
		}
		b, err := backupsPolicyClient.BackupPoliciesGet(ctx, *policyId)
		if err != nil {
			return err
		}
		if b.Model != nil {
			return fmt.Errorf("[ERROR] %s still exists after delete attempt", policyId.String())
		}
		log.Printf("[DEBUG] Deleted %s", policyId)
	}

	return nil
}
