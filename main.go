package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/jackofallops/azurerm-dalek/clients"
	"github.com/jackofallops/azurerm-dalek/dalek"
	"github.com/jackofallops/azurerm-dalek/dalek/options"
)

func main() {
	log.Print("Starting Azure Dalek..")

	prefix := flag.String("prefix", "acctest", "-prefix=acctest")
	flag.Parse()

	credentials := clients.Credentials{
		ClientID:        os.Getenv("ARM_CLIENT_ID"),
		ClientSecret:    os.Getenv("ARM_CLIENT_SECRET"),
		SubscriptionID:  os.Getenv("ARM_SUBSCRIPTION_ID"),
		TenantID:        os.Getenv("ARM_TENANT_ID"),
		EnvironmentName: os.Getenv("ARM_ENVIRONMENT"),
		Endpoint:        os.Getenv("ARM_ENDPOINT"),
	}
	opts := options.Options{
		ActuallyDelete:                 strings.EqualFold(os.Getenv("YES_I_REALLY_WANT_TO_DELETE_THINGS"), "true"),
		NumberOfResourceGroupsToDelete: int64(1000),
		Prefix:                         *prefix,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
	defer cancel()
	if err := run(ctx, credentials, opts); err != nil {
		log.Print(err.Error())
		os.Exit(1) // nolint gocritic
	}
}

func run(ctx context.Context, credentials clients.Credentials, opts options.Options) error {
	sdkClient, err := clients.BuildAzureClient(ctx, credentials)
	if err != nil {
		return fmt.Errorf("building Azure Clients: %+v", err)
	}

	log.Printf("[DEBUG] Options: %s", opts)

	client := dalek.NewDalek(sdkClient, opts)
	log.Printf("[DEBUG] Processing Resource Manager..")
	if errors := client.ResourceManager(ctx); len(errors) != 0 {
		errList := make([]string, 0)
		for _, e := range errors {
			errList = append(errList, e.Error())
		}

		return fmt.Errorf("processing Resource Manager: %+v", strings.Join(errList, "\n"))
	}

	log.Printf("[DEBUG] Processing Microsoft Graph..")
	if err := client.MicrosoftGraph(ctx); err != nil {
		return fmt.Errorf("processing Microsoft Graph: %+v", err)
	}
	log.Printf("[DEBUG] Processing Management Groups..")
	if err := client.ManagementGroups(ctx); err != nil {
		return fmt.Errorf("processing Management Groups: %+v", err)
	}

	return nil
}
