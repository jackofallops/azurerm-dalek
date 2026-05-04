package cleaners

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hashicorp/go-azure-helpers/resourcemanager/commonids"
	"github.com/hashicorp/go-azure-sdk/resource-manager/graphservices/2023-04-13/graphservicesprods"
	"github.com/jackofallops/azurerm-dalek/clients"
	"github.com/jackofallops/azurerm-dalek/dalek/options"
)

var _ ResourceGroupCleaner = graphServicesAccountCleaner{}

type graphServicesAccountCleaner struct{}

func (graphServicesAccountCleaner) Name() string {
	return "Graph Services Account"
}

func (graphServicesAccountCleaner) Cleanup(ctx context.Context, id commonids.ResourceGroupId, client *clients.AzureClient, o options.Options) error {
	c := client.ResourceManager.GraphServicesClient.Graphservicesprods

	graphServiceAccounts, err := c.AccountsListByResourceGroupComplete(ctx, id)
	if err != nil {
		return fmt.Errorf("listing Graph Service Accounts for %s: %w", id, err)
	}

	for _, g := range graphServiceAccounts.Items {
		if g.Id == nil {
			continue
		}

		graphServiceAccountID, err := graphservicesprods.ParseAccountID(*g.Id)
		if err != nil {
			return err
		}

		if !o.ActuallyDelete {
			log.Printf("would have deleted %s", graphServiceAccountID)
			continue
		}

		// For unknown reasons, Graph Service Accounts can get into a weird state where deleting them returns an internal server error
		// but if we update the account, then delete, it *usually* works so we'll attempt that here ¯\_(ツ)_/¯
		graphServiceAccount := graphservicesprods.AccountResource{
			Location: g.Location,
			Properties: graphservicesprods.AccountResourceProperties{
				AppId: g.Properties.AppId,
			},
		}

		if err := c.AccountsCreateAndUpdateThenPoll(ctx, *graphServiceAccountID, graphServiceAccount); err != nil {
			return fmt.Errorf("updating %s: %w", graphServiceAccountID, err)
		}

		// In the rare event that Azure returns a 500, this would poll until timeout, to prevent polling for ~6 hours use a new context that is much shorter.
		ctxForDelete, cancel := context.WithTimeout(ctx, time.Minute*1)
		if _, err := c.AccountsDelete(ctxForDelete, *graphServiceAccountID); err != nil {
			cancel()
			return fmt.Errorf("deleting %s: %w", graphServiceAccountID, err)
		}

		cancel()
	}

	return nil
}

func (graphServicesAccountCleaner) ResourceTypes() []string {
	return []string{
		"Microsoft.GraphServices/accounts",
	}
}
