package clients

import (
	"context"
	"fmt"
	"strings"

	dataProtection "github.com/hashicorp/go-azure-sdk/resource-manager/dataprotection/2024-04-01"
	"github.com/hashicorp/go-azure-sdk/resource-manager/eventhub/2021-11-01/disasterrecoveryconfigs"
	eventhubNamespace "github.com/hashicorp/go-azure-sdk/resource-manager/eventhub/2022-01-01-preview/namespaces"
	"github.com/hashicorp/go-azure-sdk/resource-manager/keyvault/2023-07-01/managedhsms"
	"github.com/hashicorp/go-azure-sdk/resource-manager/machinelearningservices/2024-10-01/workspaces"
	"github.com/hashicorp/go-azure-sdk/resource-manager/managementgroups/2021-04-01/managementgroups"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-05-01/capacitypools"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-05-01/netappaccounts"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-05-01/volumes"
	"github.com/hashicorp/go-azure-sdk/resource-manager/netapp/2023-05-01/volumesreplication"
	"github.com/hashicorp/go-azure-sdk/resource-manager/newrelic/2022-07-01/monitors"
	"github.com/hashicorp/go-azure-sdk/resource-manager/notificationhubs/2023-09-01/namespaces"
	paloAltoNetworks "github.com/hashicorp/go-azure-sdk/resource-manager/paloaltonetworks/2022-08-29"
	"github.com/hashicorp/go-azure-sdk/resource-manager/recoveryservices/2024-10-01/vaults"
	"github.com/hashicorp/go-azure-sdk/resource-manager/recoveryservicesbackup/2024-10-01/backupprotecteditems"
	"github.com/hashicorp/go-azure-sdk/resource-manager/recoveryservicesbackup/2024-10-01/protecteditems"
	resourceGraph "github.com/hashicorp/go-azure-sdk/resource-manager/resourcegraph/2022-10-01/resources"
	"github.com/hashicorp/go-azure-sdk/resource-manager/resources/2020-05-01/managementlocks"
	"github.com/hashicorp/go-azure-sdk/resource-manager/resources/2022-09-01/resourcegroups"
	serviceBus "github.com/hashicorp/go-azure-sdk/resource-manager/servicebus/2022-01-01-preview"
	"github.com/hashicorp/go-azure-sdk/resource-manager/storagesync/2020-03-01/cloudendpointresource"
	"github.com/hashicorp/go-azure-sdk/resource-manager/storagesync/2020-03-01/storagesyncservicesresource"
	"github.com/hashicorp/go-azure-sdk/resource-manager/storagesync/2020-03-01/syncgroupresource"
	"github.com/hashicorp/go-azure-sdk/sdk/auth"
	authWrapper "github.com/hashicorp/go-azure-sdk/sdk/auth/autorest"
	"github.com/hashicorp/go-azure-sdk/sdk/client/resourcemanager"
	"github.com/hashicorp/go-azure-sdk/sdk/environments"
	"github.com/manicminer/hamilton/msgraph"
)

type AzureClient struct {
	MicrosoftGraph  MicrosoftGraphClient
	ResourceManager ResourceManagerClient
	SubscriptionID  string
}

type MicrosoftGraphClient struct {
	Applications      *msgraph.ApplicationsClient
	Groups            *msgraph.GroupsClient
	ServicePrincipals *msgraph.ServicePrincipalsClient
	Users             *msgraph.UsersClient
}

type ResourceManagerClient struct {
	DataProtection                             *dataProtection.Client
	EventHubDisasterRecoveryClient             *disasterrecoveryconfigs.DisasterRecoveryConfigsClient
	EventHubNameSpaceClient                    *eventhubNamespace.NamespacesClient
	LocksClient                                *managementlocks.ManagementLocksClient
	MachineLearningWorkspacesClient            *workspaces.WorkspacesClient
	ManagedHSMsClient                          *managedhsms.ManagedHsmsClient
	ManagementClient                           *managementgroups.ManagementGroupsClient
	NetAppAccountClient                        *netappaccounts.NetAppAccountsClient
	NetAppCapacityPoolClient                   *capacitypools.CapacityPoolsClient
	NetAppVolumeClient                         *volumes.VolumesClient
	NetAppVolumeReplicationClient              *volumesreplication.VolumesReplicationClient
	NewRelicMonitorClient                      *monitors.MonitorsClient
	NotificationHubNamespaceClient             *namespaces.NamespacesClient
	PaloAlto                                   *paloAltoNetworks.Client
	ResourceGraphClient                        *resourceGraph.ResourcesClient
	ResourcesGroupsClient                      *resourcegroups.ResourceGroupsClient
	RecoveryServicesVaultClient                *vaults.VaultsClient
	RecoveryServicesProtectedItemClient        *protecteditems.ProtectedItemsClient
	RecoveryServicesBackupProtectedItemsClient *backupprotecteditems.BackupProtectedItemsClient
	ServiceBus                                 *serviceBus.Client
	StorageSyncClient                          *storagesyncservicesresource.StorageSyncServicesResourceClient
	StorageSyncGroupClient                     *syncgroupresource.SyncGroupResourceClient
	StorageSyncCloudEndpointClient             *cloudendpointresource.CloudEndpointResourceClient
}

type Credentials struct {
	ClientID        string
	ClientSecret    string
	SubscriptionID  string
	TenantID        string
	EnvironmentName string
	Endpoint        string
}

func BuildAzureClient(ctx context.Context, credentials Credentials) (*AzureClient, error) {
	environment, err := environmentFromCredentials(ctx, credentials)
	if err != nil {
		return nil, fmt.Errorf("determining Environment: %+v", err)
	}

	creds := auth.Credentials{
		ClientID:     credentials.ClientID,
		ClientSecret: credentials.ClientSecret,
		TenantID:     credentials.TenantID,
		Environment:  *environment,

		EnableAuthenticatingUsingClientSecret: true,
	}

	resourceManager, err := buildResourceManagerClient(ctx, creds, *environment, credentials.SubscriptionID)
	if err != nil {
		return nil, fmt.Errorf("building Resource Manager client: %+v", err)
	}

	microsoftGraph, err := buildMicrosoftGraphClient(ctx, creds, *environment)
	if err != nil {
		return nil, fmt.Errorf("building Microsoft Graph client: %+v", err)
	}

	azureClient := AzureClient{
		MicrosoftGraph:  *microsoftGraph,
		ResourceManager: *resourceManager,
		SubscriptionID:  credentials.SubscriptionID,
	}

	return &azureClient, nil
}

func environmentFromCredentials(ctx context.Context, credentials Credentials) (*environments.Environment, error) {
	if strings.Contains(strings.ToLower(credentials.EnvironmentName), "stack") {
		// for Azure Stack we have to load the Environment from the URI
		env, err := environments.FromEndpoint(ctx, credentials.Endpoint)
		if err != nil {
			return nil, fmt.Errorf("loading from Endpoint %q: %s", credentials.Endpoint, err)
		}

		return env, nil
	}

	env, err := environments.FromName(credentials.EnvironmentName)
	if err != nil {
		return nil, fmt.Errorf("loading with Name %q: %s", credentials.EnvironmentName, err)
	}

	return env, nil
}

func buildMicrosoftGraphClient(ctx context.Context, creds auth.Credentials, environment environments.Environment) (*MicrosoftGraphClient, error) {
	microsoftGraphAuthorizer, err := auth.NewAuthorizerFromCredentials(ctx, creds, environment.MicrosoftGraph)
	if err != nil {
		return nil, fmt.Errorf("building Microsoft Graph authorizer: %+v", err)
	}
	microsoftGraphEndpoint, ok := environment.MicrosoftGraph.Endpoint()
	if !ok {
		return nil, fmt.Errorf("environment %q was missing a Microsoft Graph endpoint", environment.Name)
	}

	applicationsClient := msgraph.NewApplicationsClient()
	applicationsClient.BaseClient.Authorizer = microsoftGraphAuthorizer
	applicationsClient.BaseClient.Endpoint = *microsoftGraphEndpoint

	groupsClient := msgraph.NewGroupsClient()
	groupsClient.BaseClient.Authorizer = microsoftGraphAuthorizer
	groupsClient.BaseClient.Endpoint = *microsoftGraphEndpoint

	servicePrincipalsClient := msgraph.NewServicePrincipalsClient()
	servicePrincipalsClient.BaseClient.Authorizer = microsoftGraphAuthorizer
	servicePrincipalsClient.BaseClient.Endpoint = *microsoftGraphEndpoint

	usersClient := msgraph.NewUsersClient()
	usersClient.BaseClient.Authorizer = microsoftGraphAuthorizer
	usersClient.BaseClient.Endpoint = *microsoftGraphEndpoint

	return &MicrosoftGraphClient{
		Applications:      applicationsClient,
		Groups:            groupsClient,
		ServicePrincipals: servicePrincipalsClient,
		Users:             usersClient,
	}, nil
}

func buildResourceManagerClient(ctx context.Context, creds auth.Credentials, environment environments.Environment, _ string) (*ResourceManagerClient, error) {
	resourceManagerAuthorizer, err := auth.NewAuthorizerFromCredentials(ctx, creds, environment.ResourceManager)
	if err != nil {
		return nil, fmt.Errorf("building Resource Manager authorizer: %+v", err)
	}

	autoRestAuthorizer := authWrapper.AutorestAuthorizer(resourceManagerAuthorizer)

	resourceManagerEndpoint, ok := environment.ResourceManager.Endpoint()
	if !ok {
		return nil, fmt.Errorf("environment %q was missing a Resource Manager endpoint", environment.Name)
	}

	dataProtectionClient, err := dataProtection.NewClientWithBaseURI(environment.ResourceManager, func(c *resourcemanager.Client) {
		c.Authorizer = resourceManagerAuthorizer
	})
	if err != nil {
		return nil, fmt.Errorf("building Data Protection Client: %+v", err)
	}

	eventHubDisasterRecoveryClient, err := disasterrecoveryconfigs.NewDisasterRecoveryConfigsClientWithBaseURI(environment.ResourceManager)
	if err != nil {
		return nil, fmt.Errorf("building EventHub DisasterConfigsRecovery client: %+v", err)
	}
	eventHubDisasterRecoveryClient.Client.Authorizer = resourceManagerAuthorizer

	eventHubNameSpaceClient, err := eventhubNamespace.NewNamespacesClientWithBaseURI(environment.ResourceManager)
	if err != nil {
		return nil, fmt.Errorf("building EventHubNameSpace client: %+v", err)
	}
	eventHubNameSpaceClient.Client.Authorizer = resourceManagerAuthorizer

	locksClient, err := managementlocks.NewManagementLocksClientWithBaseURI(environment.ResourceManager)
	if err != nil {
		return nil, fmt.Errorf("building ManagementLocks client: %+v", err)
	}
	locksClient.Client.Authorizer = resourceManagerAuthorizer

	workspacesClient, err := workspaces.NewWorkspacesClientWithBaseURI(environment.ResourceManager)
	if err != nil {
		return nil, fmt.Errorf("building Machine Learning Workspaces Client: %+v", err)
	}
	workspacesClient.Client.Authorizer = resourceManagerAuthorizer

	managementClient, err := managementgroups.NewManagementGroupsClientWithBaseURI(environment.ResourceManager)
	if err != nil {
		return nil, fmt.Errorf("building ManagementGroups client: %+v", err)
	}
	managementClient.Client.Authorizer = resourceManagerAuthorizer

	managedHsmsClient, err := managedhsms.NewManagedHsmsClientWithBaseURI(environment.ResourceManager)
	if err != nil {
		return nil, fmt.Errorf("building Managed HSM Client: %+v", err)
	}
	managedHsmsClient.Client.Authorizer = resourceManagerAuthorizer

	netAppAccountClient, err := netappaccounts.NewNetAppAccountsClientWithBaseURI(environment.ResourceManager)
	if err != nil {
		return nil, fmt.Errorf("building NetApp Account Client: %+v", err)
	}
	netAppAccountClient.Client.Authorizer = resourceManagerAuthorizer

	netAppCapacityPoolClient, err := capacitypools.NewCapacityPoolsClientWithBaseURI(environment.ResourceManager)
	if err != nil {
		return nil, fmt.Errorf("building NetApp Capacity Pool Client: %+v", err)
	}
	netAppCapacityPoolClient.Client.Authorizer = resourceManagerAuthorizer

	netAppVolumeClient, err := volumes.NewVolumesClientWithBaseURI(environment.ResourceManager)
	if err != nil {
		return nil, fmt.Errorf("building NetApp Volume Client: %+v", err)
	}
	netAppVolumeClient.Client.Authorizer = resourceManagerAuthorizer

	netAppVolumeReplicationClient, err := volumesreplication.NewVolumesReplicationClientWithBaseURI(environment.ResourceManager)
	if err != nil {
		return nil, fmt.Errorf("building NetApp Volume Replication Client: %+v", err)
	}
	netAppVolumeReplicationClient.Client.Authorizer = resourceManagerAuthorizer

	newRelicMonitorClient, err := monitors.NewMonitorsClientWithBaseURI(environment.ResourceManager)
	if err != nil {
		return nil, fmt.Errorf("building New Relic Monitor Client: %+v", err)
	}
	newRelicMonitorClient.Client.Authorizer = resourceManagerAuthorizer

	notificationHubNamespacesClient, err := namespaces.NewNamespacesClientWithBaseURI(environment.ResourceManager)
	if err != nil {
		return nil, fmt.Errorf("building Notification Hub Namespaces Client: %+v", err)
	}
	notificationHubNamespacesClient.Client.Authorizer = resourceManagerAuthorizer

	paloAltoClient, err := paloAltoNetworks.NewClientWithBaseURI(environment.ResourceManager, func(c *resourcemanager.Client) {
		c.Authorizer = resourceManagerAuthorizer
	})
	if err != nil {
		return nil, fmt.Errorf("building Palo Alto Networks Client: %+v", err)
	}

	recoveryServicesVaultClient, err := vaults.NewVaultsClientWithBaseURI(environment.ResourceManager)
	if err != nil {
		return nil, fmt.Errorf("building Recovery Services Vault client: %+v", err)
	}
	recoveryServicesVaultClient.Client.Authorizer = resourceManagerAuthorizer

	recoveryServicesProtectedItemClient := protecteditems.NewProtectedItemsClientWithBaseURI(*resourceManagerEndpoint)
	if err != nil {
		return nil, fmt.Errorf("building Recovery Services Protected Item client: %+v", err)
	}
	recoveryServicesProtectedItemClient.Client.Authorizer = autoRestAuthorizer

	recoveryServicesBackupProtectedItemsClient := backupprotecteditems.NewBackupProtectedItemsClientWithBaseURI(*resourceManagerEndpoint)
	if err != nil {
		return nil, fmt.Errorf("building Recovery Services Backup Protected Items client: %+v", err)
	}
	recoveryServicesBackupProtectedItemsClient.Client.Authorizer = autoRestAuthorizer

	resourceGraphClient, err := resourceGraph.NewResourcesClientWithBaseURI(environment.ResourceManager)
	if err != nil {
		return nil, fmt.Errorf("building ResourceGraph client: %+v", err)
	}
	resourceGraphClient.Client.Authorizer = resourceManagerAuthorizer

	resourcesClient, err := resourcegroups.NewResourceGroupsClientWithBaseURI(environment.ResourceManager)
	if err != nil {
		return nil, fmt.Errorf("building Resources client: %+v", err)
	}
	resourcesClient.Client.Authorizer = resourceManagerAuthorizer

	serviceBusClient, err := serviceBus.NewClientWithBaseURI(environment.ResourceManager, func(c *resourcemanager.Client) {
		c.Authorizer = resourceManagerAuthorizer
	})
	if err != nil {
		return nil, fmt.Errorf("building ServiceBus Client: %+v", err)
	}

	storageSyncClient, err := storagesyncservicesresource.NewStorageSyncServicesResourceClientWithBaseURI(environment.ResourceManager)
	if err != nil {
		return nil, fmt.Errorf("building StorageSync Client: %+v", err)
	}
	storageSyncClient.Client.Authorizer = resourceManagerAuthorizer

	storageSyncGroupClient, err := syncgroupresource.NewSyncGroupResourceClientWithBaseURI(environment.ResourceManager)
	if err != nil {
		return nil, fmt.Errorf("building StorageSyncGroup Client: %+v", err)
	}
	storageSyncGroupClient.Client.Authorizer = resourceManagerAuthorizer

	storageSyncCloudEndpointClient, err := cloudendpointresource.NewCloudEndpointResourceClientWithBaseURI(environment.ResourceManager)
	if err != nil {
		return nil, fmt.Errorf("building StorageSyncCloudEndpoint Client: %+v", err)
	}
	storageSyncCloudEndpointClient.Client.Authorizer = resourceManagerAuthorizer

	return &ResourceManagerClient{
		DataProtection:                             dataProtectionClient,
		EventHubDisasterRecoveryClient:             eventHubDisasterRecoveryClient,
		EventHubNameSpaceClient:                    eventHubNameSpaceClient,
		LocksClient:                                locksClient,
		MachineLearningWorkspacesClient:            workspacesClient,
		ManagedHSMsClient:                          managedHsmsClient,
		ManagementClient:                           managementClient,
		NetAppAccountClient:                        netAppAccountClient,
		NetAppCapacityPoolClient:                   netAppCapacityPoolClient,
		NetAppVolumeClient:                         netAppVolumeClient,
		NetAppVolumeReplicationClient:              netAppVolumeReplicationClient,
		NewRelicMonitorClient:                      newRelicMonitorClient,
		NotificationHubNamespaceClient:             notificationHubNamespacesClient,
		PaloAlto:                                   paloAltoClient,
		ResourceGraphClient:                        resourceGraphClient,
		ResourcesGroupsClient:                      resourcesClient,
		RecoveryServicesBackupProtectedItemsClient: &recoveryServicesBackupProtectedItemsClient,
		RecoveryServicesProtectedItemClient:        &recoveryServicesProtectedItemClient,
		RecoveryServicesVaultClient:                recoveryServicesVaultClient,
		ServiceBus:                                 serviceBusClient,
		StorageSyncClient:                          storageSyncClient,
		StorageSyncGroupClient:                     storageSyncGroupClient,
		StorageSyncCloudEndpointClient:             storageSyncCloudEndpointClient,
	}, nil
}
