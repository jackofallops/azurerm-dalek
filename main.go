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
	results := run(ctx, credentials, opts)
	if hasErrors(results) {
		printErrorSummary(results)
		os.Exit(1) // nolint gocritic
	}
}

type phaseResult struct {
	Phase  string
	Errors []error
}

func hasErrors(results []phaseResult) bool {
	for _, r := range results {
		if len(r.Errors) > 0 {
			return true
		}
	}
	return false
}

func printErrorSummary(results []phaseResult) {
	var total int
	for _, r := range results {
		total += len(r.Errors)
	}

	log.Print("========================================")
	log.Printf("ERROR SUMMARY (%d errors)", total)
	log.Print("========================================")
	n := 1
	for _, r := range results {
		if len(r.Errors) == 0 {
			continue
		}
		log.Printf("  %s:", r.Phase)
		for _, e := range r.Errors {
			// Replace newlines in error text to keep summary output readable
			msg := strings.ReplaceAll(e.Error(), "\n", " ")
			log.Printf("    %d. %s", n, msg)
			n++
		}
	}
	log.Print("========================================")
}

func run(ctx context.Context, credentials clients.Credentials, opts options.Options) []phaseResult {
	sdkClient, err := clients.BuildAzureClient(ctx, credentials)
	if err != nil {
		return []phaseResult{{Phase: "Initialisation", Errors: []error{fmt.Errorf("building Azure Clients: %+v", err)}}}
	}

	client := dalek.NewDalek(sdkClient, opts)

	log.Printf("[DEBUG] Options: %s", opts)

	var results []phaseResult

	log.Printf("[DEBUG] Processing Resource Manager..")
	results = append(results, phaseResult{Phase: "Resource Manager", Errors: client.ResourceManager(ctx)})

	log.Printf("[DEBUG] Processing Microsoft Graph..")
	results = append(results, phaseResult{Phase: "Microsoft Graph", Errors: client.MicrosoftGraph(ctx)})

	log.Printf("[DEBUG] Processing Management Groups..")
	results = append(results, phaseResult{Phase: "Management Groups", Errors: client.ManagementGroups(ctx)})

	return results
}
