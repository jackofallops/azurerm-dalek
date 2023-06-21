package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/tombuildsstuff/azurerm-dalek/clients"
	"github.com/tombuildsstuff/azurerm-dalek/dalek"
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
	opts := dalek.Options{
		Prefix:                         *prefix,
		NumberOfResourceGroupsToDelete: int64(1000),
		ActuallyDelete:                 strings.EqualFold(os.Getenv("YES_I_REALLY_WANT_TO_DELETE_THINGS"), "true"),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Hour)
	defer cancel()
	if err := run(ctx, credentials, opts); err != nil {
		log.Fatalf(err.Error())
	}
}

func run(ctx context.Context, credentials clients.Credentials, opts dalek.Options) error {
	sdkClient, err := clients.BuildAzureClient(ctx, credentials)
	if err != nil {
		return fmt.Errorf("building Azure Clients: %+v", err)
	}

	log.Printf("[DEBUG] Options: %s", opts)

	client := dalek.NewDalek(sdkClient, opts)
	log.Printf("[DEBUG] Processing Resource Groups..")
	if err := client.ResourceGroups(ctx); err != nil {
		return fmt.Errorf("processing Resource Groups: %+v", err)
	}
	log.Printf("[DEBUG] Processing Azure Active Directory..")
	if err := client.ActiveDirectory(ctx); err != nil {
		return fmt.Errorf("processing Azure Active Directory: %+v", err)
	}
	log.Printf("[DEBUG] Processing Management Groups..")
	if err := client.ManagementGroups(ctx); err != nil {
		return fmt.Errorf("processing Management Groups: %+v", err)
	}

	return nil
}
