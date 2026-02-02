package main

import (
	"context"
	"errors"
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
		log.Println("[DEBUG] Printing out all errors....")
		log.Print(err.Error())
		os.Exit(1) // nolint gocritic
	}
	log.Println("[DEBUG] No Errors occured...")
}

func run(ctx context.Context, credentials clients.Credentials, opts options.Options) error {
	sdkClient, err := clients.BuildAzureClient(ctx, credentials)
	if err != nil {
		return fmt.Errorf("building Azure Clients: %+v", err)
	}

	var errs []error

	log.Printf("[DEBUG] Options: %s", opts)

	client := dalek.NewDalek(sdkClient, opts)
	log.Printf("[DEBUG] Processing Resource Manager..")
	errs = append(errs, client.ResourceManager(ctx)...)

	log.Printf("[DEBUG] Processing Microsoft Graph..")
	errs = append(errs, client.MicrosoftGraph(ctx))

	log.Printf("[DEBUG] Processing Management Groups..")
	errs = append(errs, client.ManagementGroups(ctx))

	return errors.Join(errs...)
}
