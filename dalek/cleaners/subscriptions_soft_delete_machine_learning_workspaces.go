package cleaners

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/go-azure-helpers/resourcemanager/commonids"
	"github.com/hashicorp/go-azure-sdk/resource-manager/machinelearningservices/2024-10-01/workspaces"
	"github.com/jackofallops/azurerm-dalek/clients"
	"github.com/jackofallops/azurerm-dalek/dalek/options"
)

var _ SubscriptionCleaner = purgeSoftDeletedMachineLearningWorkspacesInSubscriptionCleaner{}

type purgeSoftDeletedMachineLearningWorkspacesInSubscriptionCleaner struct{}

func (p purgeSoftDeletedMachineLearningWorkspacesInSubscriptionCleaner) Name() string {
	return "Purging Soft Deleted Machine Learning Workspaces in Subscription"
}

func (p purgeSoftDeletedMachineLearningWorkspacesInSubscriptionCleaner) Cleanup(ctx context.Context, subscriptionId commonids.SubscriptionId, client *clients.AzureClient, opts options.Options) error {
	softDeletedWorkspaces, err := client.ResourceManager.MachineLearningWorkspacesClient.ListBySubscriptionComplete(ctx, subscriptionId, workspaces.DefaultListBySubscriptionOperationOptions())
	if err != nil {
		return fmt.Errorf("loading the Machine Learning Workspaces within %s: %+v", subscriptionId, err)
	}

	for _, workspace := range softDeletedWorkspaces.Items {
		workspaceId, err := workspaces.ParseWorkspaceIDInsensitively(*workspace.Id)
		if err != nil {
			return fmt.Errorf("parsing Machine Learning Workspace ID %q: %+v", *workspace.Id, err)
		}

		if !strings.HasSuffix(workspaceId.ResourceGroupName, opts.Prefix) {
			log.Printf("[DEBUG] Not deleting Machine Learning Workspace %q as it does not match target RG prefix %q", *workspaceId, opts.Prefix)
			continue
		}

		log.Printf("[DEBUG] Purging Soft-Deleted %s..", *workspaceId)
		if !opts.ActuallyDelete {
			log.Printf("[DEBUG] Would have purged soft-deleted Machine Learning Workspace %q..", *workspaceId)
			continue
		}

		purge := true
		log.Printf("[DEBUG] Purging Soft-Deleted %s..", *workspaceId)
		if err := client.ResourceManager.MachineLearningWorkspacesClient.DeleteThenPoll(ctx, *workspaceId, workspaces.DeleteOperationOptions{ForceToPurge: &purge}); err != nil {
			return fmt.Errorf("purging %s: %+v", *workspaceId, err)
		}
		log.Printf("[DEBUG] Purged Soft-Deleted %s.", *workspaceId)
	}
	return nil
}
