package clients

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/go-azure-sdk/resource-manager/keyvault/2023-02-01/managedhsms"
	"github.com/hashicorp/go-azure-sdk/resource-manager/machinelearningservices/2023-04-01-preview/workspaces"
	"github.com/hashicorp/go-azure-sdk/resource-manager/managementgroups/2021-04-01/managementgroups"
	"github.com/hashicorp/go-azure-sdk/resource-manager/paloaltonetworks/2022-08-29/certificateobjectlocalrulestack"
	"github.com/hashicorp/go-azure-sdk/resource-manager/paloaltonetworks/2022-08-29/fqdnlistlocalrulestack"
	"github.com/hashicorp/go-azure-sdk/resource-manager/paloaltonetworks/2022-08-29/localrules"
	"github.com/hashicorp/go-azure-sdk/resource-manager/paloaltonetworks/2022-08-29/localrulestacks"
	"github.com/hashicorp/go-azure-sdk/resource-manager/paloaltonetworks/2022-08-29/prefixlistlocalrulestack"
	"github.com/hashicorp/go-azure-sdk/resource-manager/resources/2020-05-01/managementlocks"
	"github.com/hashicorp/go-azure-sdk/resource-manager/resources/2022-09-01/resourcegroups"
	servicebusV20220101Preview "github.com/hashicorp/go-azure-sdk/resource-manager/servicebus/2022-01-01-preview"
	"github.com/hashicorp/go-azure-sdk/resource-manager/storagesync/2020-03-01/cloudendpointresource"
	"github.com/hashicorp/go-azure-sdk/resource-manager/storagesync/2020-03-01/storagesyncservicesresource"
	"github.com/hashicorp/go-azure-sdk/resource-manager/storagesync/2020-03-01/syncgroupresource"
	"github.com/hashicorp/go-azure-sdk/sdk/auth"
	"github.com/hashicorp/go-azure-sdk/sdk/auth/autorest"
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
	MachineLearningWorkspacesClient          *workspaces.WorkspacesClient
	LocksClient                              *managementlocks.ManagementLocksClient
	ManagementClient                         *managementgroups.ManagementGroupsClient
	ManagedHSMsClient                        *managedhsms.ManagedHsmsClient
	ResourcesClient                          *resourcegroups.ResourceGroupsClient
	ServiceBus                               *servicebusV20220101Preview.Client
	StorageSyncClient                        *storagesyncservicesresource.StorageSyncServicesResourceClient
	StorageSyncGroupClient                   *syncgroupresource.SyncGroupResourceClient
	StorageSyncCloudEndpointClient           *cloudendpointresource.CloudEndpointResourceClient
	PaloAltoLocalRulestackCertificatesClient *certificateobjectlocalrulestack.CertificateObjectLocalRulestackClient
	PaloAltoLocalRulestackFQDNClient         *fqdnlistlocalrulestack.FqdnListLocalRulestackClient
	PaloAltoLocalRulestackPrefixClient       *prefixlistlocalrulestack.PrefixListLocalRulestackClient
	PaloAltoLocalRulestacksClient            *localrulestacks.LocalRulestacksClient
	PaloAltoLocalRulestackRuleClient         *localrules.LocalRulesClient
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

	resourceManager, err := buildResourceManagerClient(ctx, creds, *environment)
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
		env, err := environments.FromEndpoint(ctx, credentials.Endpoint, credentials.EnvironmentName)
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

func buildResourceManagerClient(ctx context.Context, creds auth.Credentials, environment environments.Environment) (*ResourceManagerClient, error) {
	resourceManagerAuthorizer, err := auth.NewAuthorizerFromCredentials(ctx, creds, environment.ResourceManager)
	if err != nil {
		return nil, fmt.Errorf("building Resource Manager authorizer: %+v", err)
	}

	locksClient, err := managementlocks.NewManagementLocksClientWithBaseURI(environment.ResourceManager)
	if err != nil {
		return nil, fmt.Errorf("building Locks Client: %+v", err)
	}
	locksClient.Client.Authorizer = autorest.AutorestAuthorizer(resourceManagerAuthorizer)

	workspacesClient, err := workspaces.NewWorkspacesClientWithBaseURI(environment.ResourceManager)
	if err != nil {
		return nil, fmt.Errorf("building Machine Learning Workspaces Client: %+v", err)
	}
	workspacesClient.Client.Authorizer = autorest.AutorestAuthorizer(resourceManagerAuthorizer)

	managementClient, err := managementgroups.NewManagementGroupsClientWithBaseURI(environment.ResourceManager)
	managementClient.Client.Authorizer = autorest.AutorestAuthorizer(resourceManagerAuthorizer)

	managedHsmsClient, err := managedhsms.NewManagedHsmsClientWithBaseURI(environment.ResourceManager)
	if err != nil {
		return nil, fmt.Errorf("building Managed HSM Client: %+v", err)
	}
	managedHsmsClient.Client.Authorizer = autorest.AutorestAuthorizer(resourceManagerAuthorizer)

	paloAltoLocalRulestackCertificatesClient, err := certificateobjectlocalrulestack.NewCertificateObjectLocalRulestackClientWithBaseURI(environment.ResourceManager)
	paloAltoLocalRulestackCertificatesClient.Client.Authorizer = autorest.AutorestAuthorizer(resourceManagerAuthorizer)

	paloAltoLocalRulesClient, err := localrules.NewLocalRulesClientWithBaseURI(environment.ResourceManager)
	paloAltoLocalRulesClient.Client.Authorizer = autorest.AutorestAuthorizer(resourceManagerAuthorizer)

	paloAltoLocalRulestacksClient, err := localrulestacks.NewLocalRulestacksClientWithBaseURI(environment.ResourceManager)
	paloAltoLocalRulestacksClient.Client.Authorizer = autorest.AutorestAuthorizer(resourceManagerAuthorizer)

	paloAltoLocalRulestackFQDNClient, err := fqdnlistlocalrulestack.NewFqdnListLocalRulestackClientWithBaseURI(environment.ResourceManager)
	paloAltoLocalRulestackFQDNClient.Client.Authorizer = autorest.AutorestAuthorizer(resourceManagerAuthorizer)

	paloAltoLocalRulestackPrefixClient, err := prefixlistlocalrulestack.NewPrefixListLocalRulestackClientWithBaseURI(environment.ResourceManager)
	paloAltoLocalRulestackPrefixClient.Client.Authorizer = autorest.AutorestAuthorizer(resourceManagerAuthorizer)

	resourcesClient, err := resourcegroups.NewResourceGroupsClientWithBaseURI(environment.ResourceManager)
	if err != nil {
		return nil, fmt.Errorf("building Resources Client: %+v", err)
	}
	resourcesClient.Client.Authorizer = autorest.AutorestAuthorizer(resourceManagerAuthorizer)

	serviceBusClient, err := servicebusV20220101Preview.NewClientWithBaseURI(environment.ResourceManager, func(c *resourcemanager.Client) {
		c.Authorizer = resourceManagerAuthorizer
	})
	if err != nil {
		return nil, fmt.Errorf("building ServiceBus Client: %+v", err)
	}

	storageSyncClient, err := storagesyncservicesresource.NewStorageSyncServicesResourceClientWithBaseURI(environment.ResourceManager)
	if err != nil {
		return nil, fmt.Errorf("building StorageSync Client: %+v", err)
	}
	storageSyncClient.Client.Authorizer = autorest.AutorestAuthorizer(resourceManagerAuthorizer)

	storageSyncGroupClient, err := syncgroupresource.NewSyncGroupResourceClientWithBaseURI(environment.ResourceManager)
	if err != nil {
		return nil, fmt.Errorf("building StorageSyncGroup Client: %+v", err)
	}
	storageSyncGroupClient.Client.Authorizer = autorest.AutorestAuthorizer(resourceManagerAuthorizer)

	storageSyncCloudEndpointClient, err := cloudendpointresource.NewCloudEndpointResourceClientWithBaseURI(environment.ResourceManager)
	if err != nil {
		return nil, fmt.Errorf("building StorageSyncCloudEndpoint Client: %+v", err)
	}
	storageSyncCloudEndpointClient.Client.Authorizer = autorest.AutorestAuthorizer(resourceManagerAuthorizer)

	return &ResourceManagerClient{
		MachineLearningWorkspacesClient:          workspacesClient,
		ResourcesClient:                          resourcesClient,
		ServiceBus:                               serviceBusClient,
		LocksClient:                              locksClient,
		ManagementClient:                         managementClient,
		ManagedHSMsClient:                        managedHsmsClient,
		PaloAltoLocalRulestackCertificatesClient: paloAltoLocalRulestackCertificatesClient,
		PaloAltoLocalRulestacksClient:            paloAltoLocalRulestacksClient,
		PaloAltoLocalRulestackRuleClient:         paloAltoLocalRulesClient,
		PaloAltoLocalRulestackFQDNClient:         paloAltoLocalRulestackFQDNClient,
		PaloAltoLocalRulestackPrefixClient:       paloAltoLocalRulestackPrefixClient,
		StorageSyncClient:                        storageSyncClient,
		StorageSyncGroupClient:                   storageSyncGroupClient,
		StorageSyncCloudEndpointClient:           storageSyncCloudEndpointClient,
	}, nil
}
