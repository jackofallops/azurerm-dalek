package cleaners

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/go-azure-helpers/resourcemanager/commonids"
	"github.com/hashicorp/go-azure-sdk/resource-manager/newrelic/2022-07-01/monitors"
	"github.com/jackofallops/azurerm-dalek/clients"
	"github.com/jackofallops/azurerm-dalek/dalek/options"
)

type deleteNewRelicSubscriptionCleaner struct{}

var _ SubscriptionCleaner = deleteNewRelicSubscriptionCleaner{}

func (p deleteNewRelicSubscriptionCleaner) Name() string {
	return "Removing New Relic"
}

func (p deleteNewRelicSubscriptionCleaner) Cleanup(ctx context.Context, subscriptionId commonids.SubscriptionId, client *clients.AzureClient, opts options.Options) error {
	newRelicMonitorClient := client.ResourceManager.NewRelicMonitorClient

	monitorsLists, err := newRelicMonitorClient.ListBySubscriptionComplete(ctx, subscriptionId)
	if err != nil {
		return fmt.Errorf("listing New Relic Monitors for %s: %+v", subscriptionId, err)
	}

	for _, monitor := range monitorsLists.Items {
		if monitor.Id == nil {
			continue
		}

		monitorId, err := monitors.ParseMonitorID(*monitor.Id)
		if err != nil {
			log.Printf("[DEBUG] Parsing monitor Id %q: %+v", *monitor.Id, err)
			continue
		}

		if !opts.ActuallyDelete {
			log.Printf("[DEBUG] Would have deleted %s..", monitorId)
			continue
		}

		if monitor.Properties.UserInfo == nil || monitor.Properties.UserInfo.EmailAddress == nil {
			log.Printf("[DEBUG] `user` not found for %s..", monitorId)
			continue
		}

		if err = newRelicMonitorClient.DeleteThenPoll(ctx, *monitorId, monitors.DeleteOperationOptions{UserEmail: monitor.Properties.UserInfo.EmailAddress}); err != nil {
			log.Printf("[DEBUG] deleting %s: %+v", monitorId, err)
			continue
		}
	}

	return nil
}
