package cleaners

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/go-azure-helpers/lang/pointer"
	"github.com/hashicorp/go-azure-helpers/lang/response"
	"github.com/hashicorp/go-azure-helpers/resourcemanager/commonids"
	"github.com/hashicorp/go-azure-sdk/resource-manager/authorization/2022-04-01/roleassignments"
	"github.com/hashicorp/go-azure-sdk/resource-manager/authorization/2022-04-01/roledefinitions"
	"github.com/hashicorp/go-azure-sdk/resource-manager/resources/2022-09-01/resourcegroups"
	"github.com/hashicorp/go-azure-sdk/resource-manager/workloads/2024-09-01/sapvirtualinstances"
	"github.com/hashicorp/go-uuid"
	"github.com/jackofallops/azurerm-dalek/clients"
	"github.com/jackofallops/azurerm-dalek/dalek/options"
)

var _ ResourceGroupCleaner = sapVirtualInstance{}

type sapVirtualInstance struct{}

func (sapVirtualInstance) Name() string {
	return "SAP Virtual Instance"
}

func (sapVirtualInstance) Cleanup(ctx context.Context, id commonids.ResourceGroupId, client *clients.AzureClient, o options.Options) error {
	c := client.ResourceManager.WorkloadsClient.SAPVirtualInstances
	resourceGroupsClient := client.ResourceManager.ResourcesGroupsClient
	roleAssignmentsClient := client.ResourceManager.AuthorizationClient.RoleAssignments

	instances, err := c.ListByResourceGroupComplete(ctx, id)
	if err != nil {
		return fmt.Errorf("listing SAP Virtual Instances on %s", id)
	}

	for _, i := range instances.Items {
		if i.Id == nil {
			continue
		}

		instanceID, err := sapvirtualinstances.ParseSapVirtualInstanceID(*i.Id)
		if err != nil {
			return err
		}

		if !o.ActuallyDelete {
			log.Printf("[INFO] would have deleted %s", instanceID)
			continue
		}

		instance, err := c.Get(ctx, *instanceID)
		if err != nil {
			return err
		}

		if instance.Model == nil || instance.Model.Properties == nil {
			continue
		}

		props := instance.Model.Properties
		// If the SAP Instance has a managed resource group associated with it, we need to confirm it still exists
		if mg := props.ManagedResourceGroupConfiguration; mg != nil && mg.Name != nil {
			managedResourceGroupID := commonids.NewResourceGroupID(id.SubscriptionId, *mg.Name)

			managedResourceGroup, err := resourceGroupsClient.Get(ctx, managedResourceGroupID)
			if err != nil && !response.WasNotFound(managedResourceGroup.HttpResponse) {
				return fmt.Errorf("retrieving %s: %w", managedResourceGroupID, err)
			}

			// If the managed resource group no longer exists, we need to reprovision it, and grant access to `Azure SAP Workloads Management`
			// otherwise the deletion fails with an internal server error.
			// These will be automatically removed again by the SAP Virtual Instance deletion process.
			if response.WasNotFound(managedResourceGroup.HttpResponse) {
				resourceGroup := resourcegroups.ResourceGroup{
					Location: instance.Model.Location,
					Name:     pointer.To(managedResourceGroupID.ResourceGroupName),
				}
				if _, err := resourceGroupsClient.CreateOrUpdate(ctx, managedResourceGroupID, resourceGroup); err != nil {
					return fmt.Errorf("creating %s: %w", managedResourceGroupID, err)
				}

				roleAssignmentName, err := uuid.GenerateUUID()
				if err != nil {
					return err
				}

				scopeID := roleassignments.NewScopedRoleAssignmentID(managedResourceGroupID.ID(), roleAssignmentName)

				roleUUID := "b24988ac-6180-42a0-ab88-20f7382dd24c"    // This is the `Contributor` built-in role
				principalID := "00cc41ee-b6e1-4f2e-b3b9-b66547a967a5" // This is the `Azure SAP Workloads Management` enterprise app

				roleAssignment := roleassignments.RoleAssignmentCreateParameters{
					Properties: roleassignments.RoleAssignmentProperties{
						PrincipalId:      principalID,
						RoleDefinitionId: roledefinitions.NewScopedRoleDefinitionID(commonids.NewSubscriptionID(id.SubscriptionId).ID(), roleUUID).ID(),
					},
				}
				if _, err := roleAssignmentsClient.Create(ctx, scopeID, roleAssignment); err != nil {
					return fmt.Errorf("creating %s: %w", scopeID, err)
				}
			}
		}

		// No polling here as it could get stuck polling until the context expires when Azure errors during the async operation
		if _, err := c.Delete(ctx, *instanceID); err != nil {
			return fmt.Errorf("deleting %s: %w", *instanceID, err)
		}
	}

	return nil
}

func (sapVirtualInstance) ResourceTypes() []string {
	return []string{
		"Microsoft.Workloads/sapVirtualInstances",
	}
}
