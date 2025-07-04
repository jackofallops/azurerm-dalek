package cleaners

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/go-azure-helpers/lang/pointer"
	"github.com/hashicorp/go-azure-helpers/resourcemanager/commonids"
	"github.com/hashicorp/go-azure-sdk/resource-manager/notificationhubs/2023-09-01/namespaces"
	"github.com/hashicorp/go-azure-sdk/resource-manager/resourcegraph/2022-10-01/resources"
	"github.com/jackofallops/azurerm-dalek/clients"
	"github.com/jackofallops/azurerm-dalek/dalek/options"
)

type notificationHubNamespacesCleaner struct{}

var _ ResourceGroupCleaner = notificationHubNamespacesCleaner{}

func (c notificationHubNamespacesCleaner) Name() string {
	return "Notification Hub Namespaces"
}

func (c notificationHubNamespacesCleaner) Cleanup(ctx context.Context, id commonids.ResourceGroupId, client *clients.AzureClient, opts options.Options) error {
	// Notification Hub Namespaces don't clean up cleanly when deleting the Resource Group, so let's remove these

	log.Printf("[DEBUG] Retrieving Notification Hub Namespaces in %s..", id)
	namespaceIds, err := c.findNamespacesIDs(ctx, id, client)
	if err != nil {
		return fmt.Errorf("finding the Namespace IDs within %s: %+v", id, err)
	}

	for _, namespaceId := range *namespaceIds {
		if !opts.ActuallyDelete {
			log.Printf("[DEBUG] Would have deleted %s..", namespaceId)
			continue
		}

		log.Printf("[DEBUG] Deleting %s..", namespaceId)
		if _, err := client.ResourceManager.NotificationHubNamespaceClient.Delete(ctx, namespaceId); err != nil {
			return fmt.Errorf("deleting %s: %+v", namespaceId, err)
		}
		log.Printf("[DEBUG] Deleted %s.", namespaceId)
	}

	return nil
}

func (c notificationHubNamespacesCleaner) ResourceTypes() []string {
	return []string{
		"Microsoft.NotificationHubs/namespaces",
	}
}

func (c notificationHubNamespacesCleaner) findNamespacesIDs(ctx context.Context, resourceGroupId commonids.ResourceGroupId, client *clients.AzureClient) (*[]namespaces.NamespaceId, error) {
	query := strings.TrimSpace(fmt.Sprintf(`
resources
| where type =~ "Microsoft.NotificationHubs/namespaces"
| where resourceGroup =~ '%s'
| project id
| sort by (tolower(tostring(id))) asc
`, resourceGroupId.ResourceGroupName))
	payload := resources.QueryRequest{
		Options: &resources.QueryRequestOptions{
			Top: pointer.To(int64(1000)),
		},
		Query: query,
		Subscriptions: &[]string{
			resourceGroupId.SubscriptionId,
		},
	}
	resp, err := client.ResourceManager.ResourceGraphClient.Resources(ctx, payload)
	if err != nil {
		return nil, fmt.Errorf("performing graph query %q: %+v", query, err)
	}

	if resp.Model == nil {
		return nil, fmt.Errorf("performing graph query %q: response was nil", query)
	}
	if resp.Model.Data == nil {
		return nil, fmt.Errorf("performing graph query %q: response.data was nil", query)
	}

	namespaceIds := make([]namespaces.NamespaceId, 0)
	itemsRaw, ok := resp.Model.Data.([]interface{})
	if !ok {
		return nil, fmt.Errorf("expected the data to be an []interface but got %+v", resp.Model.Data)
	}
	for index, itemRaw := range itemsRaw {
		item, ok := itemRaw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("expected index %d to be a map[string]interface{} but it wasn't", index)
		}
		id, ok := item["id"]
		if !ok {
			return nil, fmt.Errorf("expected an id field for item %d but didn't get one", index)
		}
		idRaw := id.(string)
		namespaceId, err := namespaces.ParseNamespaceIDInsensitively(idRaw)
		if err != nil {
			return nil, fmt.Errorf("parsing %q for index %d: %+v", idRaw, index, err)
		}
		namespaceIds = append(namespaceIds, *namespaceId)
	}

	return &namespaceIds, nil
}
