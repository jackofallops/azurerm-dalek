package cleaners

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hashicorp/go-azure-helpers/lang/response"
	"github.com/hashicorp/go-azure-helpers/resourcemanager/commonids"
	"github.com/hashicorp/go-azure-sdk/resource-manager/eventhub/2021-11-01/disasterrecoveryconfigs"
	"github.com/hashicorp/go-azure-sdk/sdk/client/pollers"
	"github.com/tombuildsstuff/azurerm-dalek/clients"
	"github.com/tombuildsstuff/azurerm-dalek/dalek/options"
)

var _ ResourceGroupCleaner = eventhubNamespaceBreakPairingCleaner{}

type eventhubNamespaceBreakPairingCleaner struct {
}

func (c eventhubNamespaceBreakPairingCleaner) ResourceTypes() []string {
	return []string{
		"Microsoft.EventHub/namespaces",
	}
}

func (eventhubNamespaceBreakPairingCleaner) Name() string {
	return "EventHub Namespace - Break Pairing"
}

func (eventhubNamespaceBreakPairingCleaner) Cleanup(ctx context.Context, id commonids.ResourceGroupId, client *clients.AzureClient, opts options.Options) error {
	eventhubNamespaceClient := client.ResourceManager.EventHubNameSpaceClient
	disasterRecoveryClient := client.ResourceManager.EventHubDisasterRecoveryClient
	namespacesInResourceGroup, err := eventhubNamespaceClient.ListByResourceGroupComplete(ctx, id)
	if err != nil {
		log.Printf("[DEBUG] Error retrieving the EventHub Namespaces within %s: %+v", id, err)
	}

	for _, namespace := range namespacesInResourceGroup.Items {
		namespaceId, err := disasterrecoveryconfigs.ParseNamespaceIDInsensitively(*namespace.Id)
		if err != nil {
			log.Printf("[ERROR] Parsing EventHub Namespace ID %q: %+v", *namespace.Id, err)
			continue
		}
		log.Printf("[DEBUG] Finding Disaster Recovery Configs within %s", *namespaceId)
		configs, err := disasterRecoveryClient.ListComplete(ctx, *namespaceId)
		if err != nil {
			return fmt.Errorf("finding Disaster Recovery Configs within %s: %+v", *namespaceId, err)
		}

		for _, config := range configs.Items {
			configId, err := disasterrecoveryconfigs.ParseDisasterRecoveryConfigIDInsensitively(*config.Id)
			if err != nil {
				return fmt.Errorf("parsing the Disaster Recovery Config ID %q: %+v", *config.Id, err)
			}

			if !opts.ActuallyDelete {
				log.Printf("[DEBUG] Would have broken the pairing for %s..", *configId)
				continue
			}

			log.Printf("[DEBUG] Breaking Pairing for %s..", *configId)
			if resp, err := disasterRecoveryClient.BreakPairing(ctx, *configId); err != nil {
				if !response.WasNotFound(resp.HttpResponse) {
					return fmt.Errorf("breaking pairing for %s: %+v", *configId, err)
				}
			}
			log.Printf("[DEBUG] Polling until Pairing is broken for %s..", *configId)
			pollerType := eventhubNamespaceBreakPairingPoller{
				client:   disasterRecoveryClient,
				configId: *configId,
			}
			poller := pollers.NewPoller(pollerType, 30*time.Second, pollers.DefaultNumberOfDroppedConnectionsToAllow)
			if err := poller.PollUntilDone(ctx); err != nil {
				return fmt.Errorf("polling until the Pairing is broken for %s: %+v", *configId, err)
			}
			log.Printf("[DEBUG] Pairing Broken for %s", *configId)
		}
	}
	return nil
}

type eventhubNamespaceBreakPairingPoller struct {
	client   *disasterrecoveryconfigs.DisasterRecoveryConfigsClient
	configId disasterrecoveryconfigs.DisasterRecoveryConfigId
}

func (s eventhubNamespaceBreakPairingPoller) Poll(ctx context.Context) (*pollers.PollResult, error) {
	// poll until the status is unbroken
	result, err := s.client.Get(ctx, s.configId)
	if err != nil {
		if response.WasNotFound(result.HttpResponse) {
			return &pollers.PollResult{
				Status: pollers.PollingStatusSucceeded,
			}, nil
		}
		return nil, pollers.PollingFailedError{
			Message: err.Error(),
		}
	}

	if model := result.Model; model != nil {
		if props := model.Properties; props != nil {
			if props.PartnerNamespace == nil || *props.PartnerNamespace == "" {
				return &pollers.PollResult{
					Status: pollers.PollingStatusSucceeded,
				}, nil
			}
		}
	}

	return &pollers.PollResult{
		Status:       pollers.PollingStatusInProgress,
		PollInterval: 30 * time.Second,
	}, nil
}
