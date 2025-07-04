package cleaners

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hashicorp/go-azure-helpers/lang/pointer"
	"github.com/hashicorp/go-azure-helpers/resourcemanager/commonids"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-11-01/backuppolicy"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-11-01/backups"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-11-01/backupvaults"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-11-01/capacitypools"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-11-01/netappaccounts"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-11-01/snapshotpolicy"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-11-01/snapshots"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-11-01/volumes"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-11-01/volumesreplication"
	"github.com/hashicorp/go-azure-sdk/sdk/client"
	"github.com/hashicorp/go-azure-sdk/sdk/client/pollers"
	"github.com/hashicorp/go-azure-sdk/sdk/client/resourcemanager"
	"github.com/hashicorp/go-azure-sdk/sdk/odata"
	"github.com/jackofallops/azurerm-dalek/clients"
	"github.com/jackofallops/azurerm-dalek/dalek/options"
)

type deleteNetAppSubscriptionCleaner struct{}

var _ SubscriptionCleaner = deleteNetAppSubscriptionCleaner{}

func (p deleteNetAppSubscriptionCleaner) Name() string {
	return "Removing Net App"
}

func (p deleteNetAppSubscriptionCleaner) Cleanup(ctx context.Context, subscriptionId commonids.SubscriptionId, client *clients.AzureClient, opts options.Options) error {
	accountLists, err := client.ResourceManager.NetAppAccountClient.AccountsListBySubscription(ctx, subscriptionId)
	if err != nil || accountLists.Model == nil {
		return fmt.Errorf("listing NetApp Accounts for %s: %+v", subscriptionId, err)
	}
	log.Printf("[DEBUG] Found %d NetApp Accounts", len(*accountLists.Model))
	for _, account := range *accountLists.Model {
		accountId, err := netappaccounts.ParseNetAppAccountID(pointer.From(account.Id))
		if err != nil {
			return err
		}
		if !strings.HasPrefix(strings.ToLower(accountId.ResourceGroupName), strings.ToLower(opts.Prefix)) {
			log.Printf("[DEBUG] Not deleting %q as it does not match target RG prefix %q", accountId.ResourceGroupName, opts.Prefix)
			return nil
		}
		if err := deleteNetAppAccount(ctx, pointer.From(accountId), subscriptionId, client, opts); err != nil {
			log.Printf("deleting NetApp Account %s: %+v", pointer.From(account.Id), err)
		}
	}

	return nil
}

func deleteNetAppAccount(ctx context.Context, accountId netappaccounts.NetAppAccountId, subscriptionId commonids.SubscriptionId, client *clients.AzureClient, opts options.Options) error {
	netAppAccountClient := client.ResourceManager.NetAppAccountClient
	accountIdForBackupVault, err := backupvaults.ParseNetAppAccountID(accountId.ID())
	if err != nil {
		return fmt.Errorf("[ERROR] Unable to parse NetApp Account ID for Backup Vaults: %+v", err)
	}
	if err := deleteBackupVaults(ctx, pointer.From(accountIdForBackupVault), client, opts); err != nil {
		return err
	}

	accountIdForCapacityPool, err := capacitypools.ParseNetAppAccountID(accountId.ID())
	if err != nil {
		return fmt.Errorf("[ERROR] Unable to parse capacity pool ID: %+v", err)
	}
	if err := deleteCapacityPools(ctx, pointer.From(accountIdForCapacityPool), client, opts); err != nil {
		return err
	}

	if !opts.ActuallyDelete {
		log.Printf("[DEBUG] Would have deleted %s", accountId)
	} else {
		if _, err := netAppAccountClient.AccountsDelete(ctx, accountId); err != nil {
			return err
		}
		acctList, err := netAppAccountClient.AccountsListBySubscription(ctx, subscriptionId)
		if err == nil && acctList.Model != nil {
			for _, acct := range *acctList.Model {
				if acct.Id != nil && *acct.Id == accountId.String() {
					return fmt.Errorf("[ERROR] NetApp account %s still exists after delete attempt", accountId.String())
				}
			}
		}
		log.Printf("[DEBUG] Deleted %s", accountId)
	}
	return nil
}

func deleteCapacityPools(ctx context.Context, accountIdForCapacityPool capacitypools.NetAppAccountId, client *clients.AzureClient, opts options.Options) error {
	netAppCapacityPoolClient := client.ResourceManager.NetAppCapacityPoolClient
	capacityPoolList, err := netAppCapacityPoolClient.PoolsListComplete(ctx, accountIdForCapacityPool)
	if err != nil {
		return fmt.Errorf("listing NetApp Capacity Pools for %s: %+v", accountIdForCapacityPool, err)
	}

	log.Printf("[DEBUG] Found %d NetApp Capacity Pools", len(capacityPoolList.Items))
	for _, capacityPool := range capacityPoolList.Items {
		if capacityPool.Id == nil {
			continue
		}
		capacityPoolForVolumesId, err := volumes.ParseCapacityPoolID(pointer.From(capacityPool.Id))
		if err != nil {
			return err
		}
		if err := deleteVolumes(ctx, pointer.From(capacityPoolForVolumesId), client, opts); err != nil {
			return err
		}
		accountIdForSnapshots, err := snapshotpolicy.ParseNetAppAccountID(accountIdForCapacityPool.ID())
		if err != nil {
			return err
		}
		if err := deleteSnapshotPolicies(ctx, pointer.From(accountIdForSnapshots), client, opts); err != nil {
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
			poolList, err := netAppCapacityPoolClient.PoolsListComplete(ctx, accountIdForCapacityPool)
			if err == nil {
				for _, pool := range poolList.Items {
					if pool.Id != nil && *pool.Id == capacityPoolId.String() {
						return fmt.Errorf("[ERROR] Capacity pool %s still exists after delete attempt", capacityPoolId.String())
					}
				}
			}
			log.Printf("[DEBUG] Deleted %s", capacityPoolId)
		}
	}
	return nil
}

func deleteVolumes(ctx context.Context, capacityPoolIdForVolumes volumes.CapacityPoolId, client *clients.AzureClient, opts options.Options) error {
	netAppVolumeClient := client.ResourceManager.NetAppVolumeClient
	volumeList, err := netAppVolumeClient.ListComplete(ctx, capacityPoolIdForVolumes)
	if err != nil {
		return fmt.Errorf("listing NetApp Volumes for %s: %+v", capacityPoolIdForVolumes, err)
	}
	log.Printf("[DEBUG] Found %d NetApp Volumes", len(volumeList.Items))
	for _, volume := range volumeList.Items {
		if volume.Id == nil {
			continue
		}

		volumeIdForSnapshots, err := snapshots.ParseVolumeID(pointer.From(volume.Id))
		if err != nil {
			return err
		}
		if err := deleteSnapshots(ctx, pointer.From(volumeIdForSnapshots), client, opts); err != nil {
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

		volumeId, err := volumes.ParseVolumeID(*volume.Id)
		if err != nil {
			return err
		}
		if !opts.ActuallyDelete {
			log.Printf("[DEBUG] Would have deleted %s", volumeId)
		} else {
			forceDelete := true
			if response, err := netAppVolumeClient.Delete(ctx, *volumeId, volumes.DeleteOperationOptions{ForceDelete: &forceDelete}); err != nil {
				return err
			} else if pollerType := NewLROPoller(&lroClientAdapter{inner: netAppVolumeClient.Client}, response.HttpResponse); pollerType != nil {
				poller := pollers.NewPoller(pollerType, 0, 10)
				if err := poller.PollUntilDone(ctx); err != nil {
					return fmt.Errorf("polling delete operation for %s: %+v", volumeId, err)
				}
			}
			vol, err := netAppVolumeClient.Get(ctx, *volumeId)
			if err == nil && vol.Model != nil {
				return fmt.Errorf("[ERROR] %s still exists after delete attempt", volumeId)
			}
			log.Printf("[DEBUG] Deleted %s", volumeId)
		}
	}
	return nil
}

func deleteBackupVaults(ctx context.Context, accountIdForBackupVault backupvaults.NetAppAccountId, client *clients.AzureClient, opts options.Options) error {
	netAppBackupVaultsClient := client.ResourceManager.NetAppBackupVaultsClient
	backupVaultsList, err := netAppBackupVaultsClient.ListByNetAppAccountComplete(ctx, accountIdForBackupVault)
	if err != nil {
		return fmt.Errorf("listing NetApp Backup Vaults for %s: %+v", accountIdForBackupVault, err)
	}

	log.Printf("[DEBUG] Found %d NetApp Backup Vaults", len(backupVaultsList.Items))
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
		vaultIdForBackups, err := backups.ParseBackupVaultID(accountIdForBackupPolicy.ID())
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
		} else {
			if response, err := netAppBackupVaultsClient.Delete(ctx, *vaultIdForVault); err != nil {
				return err
			} else if pollerType := NewLROPoller(&lroClientAdapter{inner: netAppBackupVaultsClient.Client}, response.HttpResponse); pollerType != nil {
				poller := pollers.NewPoller(pollerType, 0, 10)
				if err := poller.PollUntilDone(ctx); err != nil {
					return fmt.Errorf("polling delete operation for %s: %+v", vaultIdForVault, err)
				}
			}
			vaultsList, err := netAppBackupVaultsClient.ListByNetAppAccountComplete(ctx, accountIdForBackupVault)
			if err != nil {
				return fmt.Errorf("listing NetApp Backup Vaults after deletion for %s: %+v", accountIdForBackupVault, err)
			} else {
				for _, v := range vaultsList.Items {
					if v.Id != nil && *v.Id == vaultIdForVault.String() {
						return fmt.Errorf("[ERROR] Backup vault %s still exists after delete attempt", vaultIdForVault.String())
					}
				}
			}
			log.Printf("[DEBUG] Deleted %s", vaultIdForVault)
		}
	}
	return nil
}

func deleteBackupPolicies(ctx context.Context, accountIdForBackupPolicy backuppolicy.NetAppAccountId, client *clients.AzureClient, opts options.Options) error {
	backupsPolicyClient := client.ResourceManager.NetAppBackupPolicyClient
	backupPoliciesList, err := backupsPolicyClient.BackupPoliciesList(ctx, accountIdForBackupPolicy)
	if err != nil {
		return fmt.Errorf("listing NetApp Backup Policies for %s: %+v", accountIdForBackupPolicy, err)
	}

	log.Printf("[DEBUG] Found %d NetApp Backup Policies", len(*backupPoliciesList.Model.Value))
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
				if volumeId, err := volumes.ParseVolumeID(*volume.VolumeResourceId); err != nil {
					return fmt.Errorf("parsing Volume ID %s: %+v", *volume.VolumeResourceId, err)
				} else {
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
					} else {
						log.Printf("[DEBUG] Detached volume %s from Backup Policy %s", *volume.VolumeResourceId, policyId)
					}
				}
			}
		}

		if response, err := backupsPolicyClient.BackupPoliciesDelete(ctx, *policyId); err != nil {
			return err
		} else if pollerType := NewLROPoller(&lroClientAdapter{inner: backupsPolicyClient.Client}, response.HttpResponse); pollerType != nil {
			poller := pollers.NewPoller(pollerType, 0, 10)
			if err := poller.PollUntilDone(ctx); err != nil {
				return fmt.Errorf("polling delete operation for %s: %+v", policyId, err)
			}
		}
		if b, err := backupsPolicyClient.BackupPoliciesGet(ctx, *policyId); err != nil {
			return err
		} else if b.Model != nil {
			return fmt.Errorf("[ERROR] %s still exists after delete attempt", policyId.String())
		}
		log.Printf("[DEBUG] Deleted %s", policyId)
	}

	return nil
}

func deleteSnapshots(ctx context.Context, volumeIdForSnapshots snapshots.VolumeId, client *clients.AzureClient, opts options.Options) error {
	snapshotClient := client.ResourceManager.NetAppSnapshotClient
	resp, err := snapshotClient.List(ctx, volumeIdForSnapshots)
	if err != nil || resp.Model == nil || resp.Model.Value == nil {
		return fmt.Errorf("listing NetApp Snapshots for %s", volumeIdForSnapshots)
	}
	if len(*resp.Model.Value) == 0 {
		return nil
	}
	log.Printf("[DEBUG] Found %d NetApp Snapshots", len(*resp.Model.Value))
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
		}
		if response, err := snapshotClient.Delete(ctx, *snapshotID); err != nil {
			return err
		} else if pollerType := NewLROPoller(&lroClientAdapter{inner: snapshotClient.Client}, response.HttpResponse); pollerType != nil {
			poller := pollers.NewPoller(pollerType, 0, 10)
			if err := poller.PollUntilDone(ctx); err != nil {
				return fmt.Errorf("polling delete operation for %s: %+v", snapshotID, err)
			}
		}
		log.Printf("[DEBUG] Deleted %s", snapshotID)
	}
	return nil
}

func deleteSnapshotPolicies(ctx context.Context, accountIdForSnapshots snapshotpolicy.NetAppAccountId, client *clients.AzureClient, opts options.Options) error {
	snapshotPolicyClient := client.ResourceManager.NetAppSnapshotPolicyClient
	resp, err := snapshotPolicyClient.SnapshotPoliciesList(ctx, accountIdForSnapshots)
	if err != nil || resp.Model == nil || resp.Model.Value == nil || len(*resp.Model.Value) == 0 {
		return fmt.Errorf("listing NetApp Snapshot Policies for %s: %+v", accountIdForSnapshots, err)
	}
	log.Printf("[DEBUG] Found %d NetApp Snapshot Policies", len(*resp.Model.Value))
	for _, snapshotPolicy := range *resp.Model.Value {
		if snapshotPolicy.Id == nil {
			continue
		}
		policyID, err := snapshotpolicy.ParseSnapshotPolicyID(*snapshotPolicy.Id)
		if err != nil {
			return err
		}
		if !opts.ActuallyDelete {
			log.Printf("[DEBUG] Would have deleted %s", policyID.String())
			continue
		} else {
			if response, err := snapshotPolicyClient.SnapshotPoliciesDelete(ctx, *policyID); err != nil {
				return err
			} else if pollerType := NewLROPoller(&lroClientAdapter{inner: snapshotPolicyClient.Client}, response.HttpResponse); pollerType != nil {
				poller := pollers.NewPoller(pollerType, 0, 10)
				if err := poller.PollUntilDone(ctx); err != nil {
					return fmt.Errorf("polling delete operation for %s: %+v", policyID, err)
				}
			}
		}
		log.Printf("[DEBUG] Deleted %s", policyID)
	}
	return nil
}

func deleteBackups(ctx context.Context, backupsVaultId backups.BackupVaultId, client *clients.AzureClient, opts options.Options) error {
	netAppBackupsClient := client.ResourceManager.NetAppBackupsClient
	backupsList, err := netAppBackupsClient.ListByVaultComplete(ctx, backupsVaultId, backups.ListByVaultOperationOptions{})
	if err != nil {
		return err
	}
	log.Printf("[DEBUG] Found %d NetApp Backups", len(backupsList.Items))
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
		if response, err := netAppBackupsClient.Delete(ctx, *backupId); err != nil {
			return err
		} else if pollerType := NewLROPoller(&lroClientAdapter{inner: netAppBackupsClient.Client}, response.HttpResponse); pollerType != nil {
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

var _ pollers.PollerType = &netappLROPoller{}

type netappLROPoller struct {
	client              LROClient
	azureAsyncOperation string
}

var (
	pollingSuccess = pollers.PollResult{
		Status:       pollers.PollingStatusSucceeded,
		PollInterval: 10 * time.Second,
	}
	pollingInProgress = pollers.PollResult{
		Status:       pollers.PollingStatusInProgress,
		PollInterval: 10 * time.Second,
	}
)

type LROClient interface {
	NewRequest(ctx context.Context, opts client.RequestOptions) (*http.Request, error)
	Execute(ctx context.Context, req *http.Request) (*http.Response, error)
}

func NewLROPoller(client LROClient, response *http.Response) *netappLROPoller {
	if urlStr := response.Header.Get("Azure-AsyncOperation"); urlStr != "" {
		return &netappLROPoller{
			client:              client,
			azureAsyncOperation: urlStr,
		}
	}
	return nil
}

type myOptions struct {
	azureAsyncOperation string
}

var _ client.Options = myOptions{}

func (p myOptions) ToHeaders() *client.Headers {
	return &client.Headers{}
}

func (p myOptions) ToOData() *odata.Query {
	return &odata.Query{}
}

func (p myOptions) ToQuery() *client.QueryParams {
	u, err := url.Parse(p.azureAsyncOperation)
	if err != nil {
		log.Printf("[ERROR] Unable to parse Azure-AsyncOperation URL: %v", err)
		return nil
	}
	q := client.QueryParams{}
	for k, v := range u.Query() {
		if len(v) > 0 {
			q.Append(k, v[0])
		}
	}
	return &q
}

func (p netappLROPoller) Poll(ctx context.Context) (*pollers.PollResult, error) {
	if p.azureAsyncOperation == "" {
		return &pollingSuccess, nil
	}
	p.azureAsyncOperation = strings.Replace(p.azureAsyncOperation, "https://management.azure.com/", "", 1)
	opts := client.RequestOptions{
		ContentType: "application/json; charset=utf-8",
		ExpectedStatusCodes: []int{
			http.StatusOK,
			http.StatusAccepted,
		},
		HttpMethod: http.MethodGet,
		Path:       p.azureAsyncOperation,
		OptionsObject: myOptions{
			azureAsyncOperation: p.azureAsyncOperation,
		},
	}
	req, err := p.client.NewRequest(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("building request: %+v", err)
	}
	resp, err := p.client.Execute(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("getting status: %+v", err)
	}
	var respBody pollingResponse
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return nil, fmt.Errorf("decoding response body: %+v", err)
	}
	if respBody.Status == "Deleting" {
		return &pollingInProgress, nil
	}
	if respBody.Status == "Failed" {
		return nil, pollers.PollingFailedError{
			Message: respBody.Error.Message,
		}
	}
	if respBody.Status == "Succeeded" {
		return &pollingSuccess, nil
	}

	return nil, fmt.Errorf("unexpected status code %d. Response body: %s", resp.StatusCode, resp.Body)
}

type pollingResponse struct {
	Status string `json:"status"`
	Error  struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// lroClientAdapter adapts a *resourcemanager.Client to the LROClient interface expected by the poller.
type lroClientAdapter struct {
	inner *resourcemanager.Client
}

func (a *lroClientAdapter) NewRequest(ctx context.Context, opts client.RequestOptions) (*http.Request, error) {
	cReq, err := a.inner.NewRequest(ctx, opts)
	if err != nil {
		return nil, err
	}
	return cReq.Request, nil
}

func (a *lroClientAdapter) Execute(ctx context.Context, req *http.Request) (*http.Response, error) {
	// Wrap the http.Request in a client.Request
	cReq := &client.Request{Request: req, Client: a.inner}
	resp, err := a.inner.Execute(ctx, cReq)
	if err != nil {
		return nil, err
	}
	return resp.Response, nil
}
