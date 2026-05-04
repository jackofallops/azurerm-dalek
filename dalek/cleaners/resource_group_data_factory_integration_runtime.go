package cleaners

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/go-azure-helpers/resourcemanager/commonids"
	"github.com/hashicorp/go-azure-sdk/resource-manager/datafactory/2018-06-01/factories"
	"github.com/hashicorp/go-azure-sdk/resource-manager/datafactory/2018-06-01/integrationruntimes"
	"github.com/jackofallops/azurerm-dalek/clients"
	"github.com/jackofallops/azurerm-dalek/dalek/options"
)

var _ ResourceGroupCleaner = dataFactoryCleaner{}

type dataFactoryCleaner struct{}

func (c dataFactoryCleaner) Name() string {
	return "Remove Data Factory instances"
}

func (c dataFactoryCleaner) Cleanup(ctx context.Context, id commonids.ResourceGroupId, client *clients.AzureClient, o options.Options) error {
	dfClient := client.ResourceManager.DataFactory

	dataFactories, err := dfClient.Factories.ListByResourceGroupComplete(ctx, id)
	if err != nil {
		return fmt.Errorf("listing Data Factories for %s", id)
	}

	for _, d := range dataFactories.Items {
		if d.Id == nil {
			continue
		}

		dataFactoryID, err := integrationruntimes.ParseFactoryIDInsensitively(*d.Id)
		if err != nil {
			return err
		}

		integrationRuntimes, err := dfClient.IntegrationRuntimes.ListByFactoryComplete(ctx, *dataFactoryID)
		if err != nil {
			return fmt.Errorf("listing Integration Runtimes for %s", dataFactoryID)
		}

		for _, i := range integrationRuntimes.Items {
			if i.Id == nil {
				continue
			}

			integrationRuntimeID, err := integrationruntimes.ParseIntegrationRuntimeIDInsensitively(*i.Id)
			if err != nil {
				return err
			}

			if !o.ActuallyDelete {
				log.Printf("[INFO] would have deleted %s", integrationRuntimeID)
				continue
			}

			if _, err := dfClient.IntegrationRuntimes.Delete(ctx, *integrationRuntimeID); err != nil {
				return fmt.Errorf("deleting %s", integrationRuntimeID)
			}
		}

		if !o.ActuallyDelete {
			log.Printf("[INFO] would have deleted %s", dataFactoryID)
			return nil
		}

		if _, err := dfClient.Factories.Delete(ctx, factories.FactoryId(*dataFactoryID)); err != nil {
			return fmt.Errorf("deleting %s", dataFactoryID)
		}
	}

	return nil
}

func (c dataFactoryCleaner) ResourceTypes() []string {
	return []string{
		"Microsoft.DataFactory/factories",
	}
}
