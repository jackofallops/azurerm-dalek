package cleaners

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/go-azure-helpers/lang/pointer"
	"github.com/hashicorp/go-azure-helpers/resourcemanager/commonids"
	"github.com/hashicorp/go-azure-sdk/resource-manager/network/2024-05-01/subnets"
	"github.com/hashicorp/go-azure-sdk/resource-manager/web/2024-04-01/resourceproviders"
	webResourceProviders "github.com/hashicorp/go-azure-sdk/resource-manager/web/2024-04-01/resourceproviders"
	baseSdkClient "github.com/hashicorp/go-azure-sdk/sdk/client"
	"github.com/jackofallops/azurerm-dalek/clients"
	"github.com/jackofallops/azurerm-dalek/dalek/options"
)

var _ ResourceGroupCleaner = networkSubnetPropertiesCleaner{}

type networkSubnetPropertiesCleaner struct{}

func (networkSubnetPropertiesCleaner) Name() string {
	return "Remove Network Subnet Options.."
}

func (networkSubnetPropertiesCleaner) Cleanup(ctx context.Context, id commonids.ResourceGroupId, client *clients.AzureClient, opts options.Options) error {
	networkList, err := client.ResourceManager.NetworkClient.List(ctx, id)
	if err != nil {
		return fmt.Errorf("retrieving networks for resource group %s: %+v", id, err)
	}

	if networkList.Model != nil {
		for _, net := range *networkList.Model {
			networkId, err := commonids.ParseVirtualNetworkID(*net.Id)
			if err != nil {
				return fmt.Errorf("parsing Virtual Network ID %s: %+v", *net.Id, err)
			}

			subnetList, err := client.ResourceManager.NetworkSubnetClient.List(ctx, *networkId)
			if err != nil {
				return fmt.Errorf("retrieving subnets for resource group %s: %+v", id, err)
			}

			if subnetList.Model != nil {
				for _, sub := range *subnetList.Model {
					// Updates/deletes are not allowed on a subnet if there are existing orphan network integragions.
					// Call a special purge API that will clean up any integrations before we attempt to update.
					fmt.Printf("[DEBUG]   Purging unused network integrations for Network Subnet %s", *sub.Id)

					webProviderLocationId := resourceproviders.ProviderLocationId{
						SubscriptionId: id.SubscriptionId,
						LocationName:   *net.Location,
					}

					err := purgeUnusedVnetIntegrations(ctx, webProviderLocationId, *sub.Id, client.ResourceManager.WebResourceProviderClient)
					if err != nil {
						return fmt.Errorf("purging unused network integrations for Subnet %s: %+v", *sub.Id, err)
					}

					fmt.Printf("[DEBUG]   Updating default properties for Network Subnet %s", *sub.Id)

					subnetId, err := commonids.ParseSubnetID(*sub.Id)
					if err != nil {
						return fmt.Errorf("parsing Subnet ID %s: %+v", *sub.Id, err)
					}

					// update delegation and private policies to their default values (disabled)
					sub.Properties.Delegations = pointer.To([]subnets.Delegation{})
					sub.Properties.PrivateEndpointNetworkPolicies = pointer.To(subnets.VirtualNetworkPrivateEndpointNetworkPoliciesDisabled)

					if _, err := client.ResourceManager.NetworkSubnetClient.CreateOrUpdate(ctx, *subnetId, sub); err != nil {
						return fmt.Errorf("updating properties for Subnet %s: %+v", subnetId, err)
					}
				}
			}
		}
	}
	return nil
}

func (networkSubnetPropertiesCleaner) ResourceTypes() []string {
	return []string{
		"Microsoft.Network/virtualNetworks",
	}
}

// Deletion or updating of subnets can fail if there are existing Service Association Links (SAL).
// It is possible that SALs can be orphans, where the service that was associated has been destroyed but SAL remains.
// Based on the following article you can remove orphaned SALs with a special API:
// https://learn.microsoft.com/en-us/azure/app-service/overview-vnet-integration#method-1-purging-orphaned-service-association-links-sal
// The purge API has not been added to official swagger, so does not exist as client in SDK.
// This helper function will use existing Web ResourceProviders SDK to call the appropriate API.
func purgeUnusedVnetIntegrations(ctx context.Context, locationId webResourceProviders.ProviderLocationId, subnetId string, client *webResourceProviders.ResourceProvidersClient) (err error) {
	opts := baseSdkClient.RequestOptions{
		ContentType: "application/json; charset=utf-8",
		ExpectedStatusCodes: []int{
			http.StatusOK,
		},
		HttpMethod: http.MethodPost,
		Path:       fmt.Sprintf("%s/purgeUnusedVirtualNetworkIntegration", locationId.ID()),
	}

	type vnetIntegrationRequest struct {
		SubnetResourceId string `json:"subnetResourceId"`
	}

	input := vnetIntegrationRequest{
		SubnetResourceId: subnetId,
	}

	req, err := client.Client.NewRequest(ctx, opts)
	if err != nil {
		return
	}

	if err = req.Marshal(input); err != nil {
		return
	}

	// do not need the response here, just the status handled by the error condition
	_, err = req.Execute(ctx)

	if err != nil {
		return
	}

	return
}
