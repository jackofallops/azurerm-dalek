package cleaners

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/go-azure-helpers/lang/pointer"
	"github.com/hashicorp/go-azure-helpers/resourcemanager/commonids"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-11-01/backupvaults"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-11-01/capacitypools"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-11-01/netappaccounts"
	"github.com/hashicorp/go-azure-sdk/sdk/client/pollers"
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
	log.Printf("[DEBUG] Found %d NetApp Accounts in %s", len(*accountLists.Model), subscriptionId)
	for _, account := range *accountLists.Model {
		accountId, err := netappaccounts.ParseNetAppAccountID(pointer.From(account.Id))
		if err != nil {
			log.Println(err)
			continue
		}
		if !strings.HasPrefix(strings.ToLower(accountId.ResourceGroupName), strings.ToLower(opts.Prefix)) {
			log.Printf("[DEBUG] Not deleting %q as it does not match target RG prefix %q", accountId.ResourceGroupName, opts.Prefix)
			continue
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
		return nil
	}
	resp, err := netAppAccountClient.AccountsDelete(ctx, accountId)
	if err != nil {
		return err
	}
	pollerType, err := NewLROPoller(&lroClientAdapter{inner: netAppAccountClient.Client}, resp.HttpResponse)
	if err != nil {
		return fmt.Errorf("creating poller for %s: %+v", accountId, err)
	}
	if pollerType != nil {
		poller := pollers.NewPoller(pollerType, 0, 10)
		if err := poller.PollUntilDone(ctx); err != nil {
			return fmt.Errorf("polling delete operation for %s: %+v", accountId, err)
		}
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

	return nil
}
