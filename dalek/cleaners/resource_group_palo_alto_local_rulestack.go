package cleaners

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/hashicorp/go-azure-helpers/lang/pointer"
	"github.com/hashicorp/go-azure-helpers/lang/response"
	"github.com/hashicorp/go-azure-helpers/resourcemanager/commonids"
	"github.com/hashicorp/go-azure-sdk/resource-manager/paloaltonetworks/2022-08-29/certificateobjectlocalrulestack"
	"github.com/hashicorp/go-azure-sdk/resource-manager/paloaltonetworks/2022-08-29/fqdnlistlocalrulestack"
	"github.com/hashicorp/go-azure-sdk/resource-manager/paloaltonetworks/2022-08-29/localrules"
	"github.com/hashicorp/go-azure-sdk/resource-manager/paloaltonetworks/2022-08-29/localrulestacks"
	"github.com/hashicorp/go-azure-sdk/resource-manager/paloaltonetworks/2022-08-29/prefixlistlocalrulestack"
	"github.com/jackofallops/azurerm-dalek/clients"
	"github.com/jackofallops/azurerm-dalek/dalek/options"
)

type paloAltoLocalRulestackCleaner struct{}

var _ ResourceGroupCleaner = paloAltoLocalRulestackCleaner{}

func (paloAltoLocalRulestackCleaner) Name() string {
	return "Removing Rulestack Rules"
}

func (paloAltoLocalRulestackCleaner) Cleanup(ctx context.Context, id commonids.ResourceGroupId, client *clients.AzureClient, opts options.Options) error {
	rulestacksClient := client.ResourceManager.PaloAlto.LocalRulestacks

	rulestacks, err := rulestacksClient.ListByResourceGroupComplete(ctx, id)
	if err != nil {
		log.Printf("[DEBUG] Error retrieving the Palo Alto Local Rulestacks within %s: %+v", id, err)
	}

	errs := make([]error, 0)

	// Rules
	rulesClient := client.ResourceManager.PaloAlto.LocalRules
	for _, rg := range rulestacks.Items {
		rulestackId := localrules.NewLocalRulestackID(id.SubscriptionId, id.ResourceGroupName, pointer.From(rg.Name))
		rulesInRulestack, err := rulesClient.ListByLocalRulestacks(ctx, rulestackId)
		if err != nil {
			if response.WasStatusCode(rulesInRulestack.HttpResponse, 500) || response.WasNotFound(rulesInRulestack.HttpResponse) || response.WasStatusCode(rulesInRulestack.HttpResponse, 502) {
				continue
			}
			errs = append(errs, fmt.Errorf("listing rules for %s: %+v", rulestackId, err))
			continue
		}
		if model := rulesInRulestack.Model; model != nil {
			for _, v := range *model {
				ruleId, err := localrules.ParseLocalRuleIDInsensitively(pointer.From(v.Id))
				if err != nil {
					errs = append(errs, fmt.Errorf("parsing rule %s: %+v", pointer.From(v.Id), err))
					continue
				}

				if !opts.ActuallyDelete {
					log.Printf("[DEBUG] Would have deleted the Local Rule for %s..", *ruleId)
					continue
				}

				log.Printf("[DEBUG] Deleting %s..", *ruleId)
				if _, err := rulesClient.Delete(ctx, *ruleId); err != nil {
					log.Printf("[ERROR] deleting rule %s from rulestack %s: %+v", ruleId, rulestackId, err)
					log.Printf("[DEBUG] Support ticket required to remove %s", rulestackId)
					errs = append(errs, fmt.Errorf("deleting rule %s from rulestack %s: %+v", ruleId, rulestackId, err))
					continue
				}
				log.Printf("[DEBUG] Deleted %s..", *ruleId)
			}
		}
		if _, err := rulestacksClient.Commit(ctx, localrulestacks.NewLocalRulestackID(rulestackId.SubscriptionId, rulestackId.ResourceGroupName, rulestackId.LocalRulestackName)); err != nil {
			errs = append(errs, fmt.Errorf("committing rule changes to %s: %+v", rulestackId, err))
		}
	}

	// FQDN Lists
	fqdnClient := client.ResourceManager.PaloAlto.FqdnListLocalRulestack
	for _, rg := range rulestacks.Items {
		rulestackId := fqdnlistlocalrulestack.NewLocalRulestackID(id.SubscriptionId, id.ResourceGroupName, pointer.From(rg.Name))
		fqdnInRulestack, err := fqdnClient.ListByLocalRulestacks(ctx, rulestackId)
		if err != nil {
			if response.WasStatusCode(fqdnInRulestack.HttpResponse, 500) || response.WasStatusCode(fqdnInRulestack.HttpResponse, 502) || response.WasNotFound(fqdnInRulestack.HttpResponse) {
				continue
			}
			errs = append(errs, fmt.Errorf("listing FQDNs for %s: %+v", rulestackId, err))
			continue
		}
		if model := fqdnInRulestack.Model; model != nil {
			for _, v := range *model {
				fqdnId, err := fqdnlistlocalrulestack.ParseLocalRulestackFqdnListIDInsensitively(pointer.From(v.Id))
				if err != nil {
					errs = append(errs, fmt.Errorf("parsing %q as a fqdn list id: %+v", pointer.From(v.Id), err))
					continue
				}

				if !opts.ActuallyDelete {
					log.Printf("[DEBUG] Would have deleted the FQDN for %s..", *fqdnId)
					continue
				}

				log.Printf("[DEBUG] Deleting %s..", *fqdnId)
				if _, err := fqdnClient.Delete(ctx, *fqdnId); err != nil {
					log.Printf("[ERROR] deleting fqdn %s from rulestack %s: %+v", fqdnId, rulestackId, err)
					log.Printf("[DEBUG] Support ticket required to remove %s", rulestackId)
					errs = append(errs, fmt.Errorf("deleting fqdn %s from rulestack %s: %+v", fqdnId, rulestackId, err))
					continue
				}
				log.Printf("[DEBUG] Deleted %s..", *fqdnId)
			}
		}
		if _, err := rulestacksClient.Commit(ctx, localrulestacks.NewLocalRulestackID(rulestackId.SubscriptionId, rulestackId.ResourceGroupName, rulestackId.LocalRulestackName)); err != nil {
			errs = append(errs, fmt.Errorf("committing FQDN changes to %s: %+v", rulestackId, err))
		}
	}

	// Certificates
	certClient := client.ResourceManager.PaloAlto.CertificateObjectLocalRulestack
	for _, rg := range rulestacks.Items {
		// Remove inspection config - blocks removal of certs if referenced
		rulestackId := certificateobjectlocalrulestack.NewLocalRulestackID(id.SubscriptionId, id.ResourceGroupName, pointer.From(rg.Name))
		rs, err := rulestacksClient.Get(ctx, localrulestacks.LocalRulestackId(rulestackId))
		if err != nil {
			errs = append(errs, fmt.Errorf("getting rulestack %s: %+v", rulestackId, err))
			continue
		}
		sec := pointer.From(rs.Model.Properties.SecurityServices)
		if pointer.From(sec.OutboundTrustCertificate) != "" || pointer.From(sec.OutboundUnTrustCertificate) != "" {
			sec.OutboundTrustCertificate = nil
			sec.OutboundUnTrustCertificate = nil
			rs.Model.Properties.SecurityServices = pointer.To(sec)
			localRulestackId := localrulestacks.NewLocalRulestackID(rulestackId.SubscriptionId, rulestackId.ResourceGroupName, rulestackId.LocalRulestackName)
			if err = rulestacksClient.CreateOrUpdateThenPoll(ctx, localRulestackId, *rs.Model); err != nil {
				errs = append(errs, fmt.Errorf("removing certificate usage on %s: %+v", rulestackId, err))
			}
		}
		// Remove certs
		certInRulestack, err := certClient.ListByLocalRulestacks(ctx, rulestackId)
		if err != nil {
			if response.WasStatusCode(certInRulestack.HttpResponse, 500) || response.WasStatusCode(certInRulestack.HttpResponse, 502) || response.WasNotFound(certInRulestack.HttpResponse) {
				continue
			}
			errs = append(errs, fmt.Errorf("listing certificates for %s: %+v", rulestackId, err))
			continue
		}
		if model := certInRulestack.Model; model != nil {
			for _, v := range *model {
				certId, err := certificateobjectlocalrulestack.ParseLocalRulestackCertificateID(pointer.From(v.Id))
				if err != nil {
					errs = append(errs, fmt.Errorf("parsing %q as a certificate id: %+v", pointer.From(v.Id), err))
					continue
				}

				if !opts.ActuallyDelete {
					log.Printf("[DEBUG] Would have deleted the Certificate for %s..", *certId)
					continue
				}

				log.Printf("[DEBUG] Deleting %s..", *certId)
				if _, err := certClient.Delete(ctx, *certId); err != nil {
					log.Printf("[ERROR] deleting certificate %s from rulestack %s: %+v", certId, rulestackId, err)
					log.Printf("[DEBUG] Support ticket required to remove %s", rulestackId)
					errs = append(errs, fmt.Errorf("deleting certificate %s from rulestack %s: %+v", certId, rulestackId, err))
					continue
				}
				log.Printf("[DEBUG] Deleted %s..", *certId)
			}
		}
		if _, err := rulestacksClient.Commit(ctx, localrulestacks.NewLocalRulestackID(rulestackId.SubscriptionId, rulestackId.ResourceGroupName, rulestackId.LocalRulestackName)); err != nil {
			errs = append(errs, fmt.Errorf("committing certificate changes to %s: %+v", rulestackId, err))
		}
	}

	// Prefixes
	prefixClient := client.ResourceManager.PaloAlto.PrefixListLocalRulestack
	for _, rg := range rulestacks.Items {
		rulestackId := prefixlistlocalrulestack.NewLocalRulestackID(id.SubscriptionId, id.ResourceGroupName, pointer.From(rg.Name))
		prefixInRulestack, err := prefixClient.ListByLocalRulestacks(ctx, rulestackId)
		if err != nil {
			if response.WasStatusCode(prefixInRulestack.HttpResponse, 500) || response.WasStatusCode(prefixInRulestack.HttpResponse, 502) || response.WasNotFound(prefixInRulestack.HttpResponse) {
				continue
			}
			errs = append(errs, fmt.Errorf("listing prefixes for %s: %+v", rulestackId, err))
			continue
		}
		if model := prefixInRulestack.Model; model != nil {
			for _, v := range *model {
				prefixId, err := prefixlistlocalrulestack.ParseLocalRulestackPrefixListIDInsensitively(pointer.From(v.Id))
				if err != nil {
					errs = append(errs, fmt.Errorf("parsing %q as a prefix list id: %+v", pointer.From(v.Id), err))
					continue
				}

				if !opts.ActuallyDelete {
					log.Printf("[DEBUG] Would have deleted the Prefix for %s..", *prefixId)
					continue
				}

				log.Printf("[DEBUG] Deleting %s..", *prefixId)
				if _, err := prefixClient.Delete(ctx, *prefixId); err != nil {
					log.Printf("[ERROR] deleting prefix %s from rulestack %s: %+v", prefixId, rulestackId, err)
					log.Printf("[DEBUG] Support ticket required to remove %s", rulestackId)
					errs = append(errs, fmt.Errorf("deleting prefix %s from rulestack %s: %+v", prefixId, rulestackId, err))
					continue
				}
				log.Printf("[DEBUG] Deleted %s..", *prefixId)
			}
		}
		if _, err := rulestacksClient.Commit(ctx, localrulestacks.NewLocalRulestackID(rulestackId.SubscriptionId, rulestackId.ResourceGroupName, rulestackId.LocalRulestackName)); err != nil {
			errs = append(errs, fmt.Errorf("committing prefix changes to %s: %+v", rulestackId, err))
		}
	}

	return errors.Join(errs...)
}

func (paloAltoLocalRulestackCleaner) ResourceTypes() []string {
	return []string{
		"PaloAltoNetworks.Cloudngfw/firewalls",
		"PaloAltoNetworks.Cloudngfw/localRulestacks",
		"PaloAltoNetworks.Cloudngfw/localRulestacks/certificates",
		"PaloAltoNetworks.Cloudngfw/localRulestacks/fqdnLists",
		"PaloAltoNetworks.Cloudngfw/localRulestacks/localRules",
		"PaloAltoNetworks.Cloudngfw/localRulestacks/prefixLists",
		"PaloAltoNetworks.Cloudngfw/globalRulestacks",
		"PaloAltoNetworks.Cloudngfw/globalRulestacks/certificates",
		"PaloAltoNetworks.Cloudngfw/globalRulestacks/fqdnLists",
		"PaloAltoNetworks.Cloudngfw/globalRulestacks/postRules",
		"PaloAltoNetworks.Cloudngfw/globalRulestacks/preRules",
		"PaloAltoNetworks.Cloudngfw/globalRulestacks/prefixLists",
	}
}
