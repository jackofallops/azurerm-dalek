package cleaners

import (
	"context"

	"github.com/hashicorp/go-azure-helpers/resourcemanager/commonids"
	"github.com/tombuildsstuff/azurerm-dalek/clients"
	"github.com/tombuildsstuff/azurerm-dalek/dalek/options"
)

var SubscriptionCleaners = []SubscriptionCleaner{
	// deleteNetAppSubscriptionCleaner{}, // TODO - This cleaner is exiting the whole run on first error, needs looking into.
	deleteStorageSyncSubscriptionCleaner{},
	deleteResourceGroupsInSubscriptionCleaner{},
	purgeSoftDeletedManagedHSMsInSubscriptionCleaner{},
	purgeSoftDeletedMachineLearningWorkspacesInSubscriptionCleaner{},
}

type SubscriptionCleaner interface {
	// Name specifies the name of this SubscriptionCleaner
	Name() string

	// Cleanup performs this clean-up operation against the given Subscription
	Cleanup(ctx context.Context, subscriptionId commonids.SubscriptionId, client *clients.AzureClient, opts options.Options) error
}
