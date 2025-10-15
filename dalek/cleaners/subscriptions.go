package cleaners

import (
	"context"

	"github.com/hashicorp/go-azure-helpers/resourcemanager/commonids"
	"github.com/jackofallops/azurerm-dalek/clients"
	"github.com/jackofallops/azurerm-dalek/dalek/options"
)

var SubscriptionCleaners = []SubscriptionCleaner{
	deleteNetAppSubscriptionCleaner{},
	// deleteRecoveryServicesVaultSubscriptionCleaner{},
	// deleteNewRelicSubscriptionCleaner{},
	// deleteStorageSyncSubscriptionCleaner{},
	// deleteResourceGroupsInSubscriptionCleaner{},
	// purgeSoftDeletedManagedHSMsInSubscriptionCleaner{},
	// purgeSoftDeletedMachineLearningWorkspacesInSubscriptionCleaner{},
}

type SubscriptionCleaner interface {
	// Name specifies the name of this SubscriptionCleaner
	Name() string

	// Cleanup performs this clean-up operation against the given Subscription
	Cleanup(ctx context.Context, subscriptionId commonids.SubscriptionId, client *clients.AzureClient, opts options.Options) error
}
