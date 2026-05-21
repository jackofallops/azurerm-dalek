package cleaners

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/go-azure-helpers/lang/pointer"
	"github.com/hashicorp/go-azure-helpers/resourcemanager/commonids"
	"github.com/hashicorp/go-azure-sdk/resource-manager/compute/2022-03-03/galleries"
	"github.com/hashicorp/go-azure-sdk/resource-manager/compute/2022-03-03/gallerysharingupdate"
	"github.com/jackofallops/azurerm-dalek/clients"
	"github.com/jackofallops/azurerm-dalek/dalek/options"
)

var _ ResourceGroupCleaner = computeGalleryCleaner{}

type computeGalleryCleaner struct{}

func (c computeGalleryCleaner) Name() string {
	return "Compute Galleries"
}

func (c computeGalleryCleaner) Cleanup(ctx context.Context, id commonids.ResourceGroupId, client *clients.AzureClient, o options.Options) error {
	computeClient := client.ResourceManager.ComputeClient

	computeGalleries, err := computeClient.Galleries.ListByResourceGroupComplete(ctx, id)
	if err != nil {
		return fmt.Errorf("listing Compute Galleries for %s", id)
	}

	for _, g := range computeGalleries.Items {
		if g.Id == nil {
			continue
		}

		if g.Properties == nil || g.Properties.SharingProfile == nil || pointer.From(g.Properties.SharingProfile.Permissions) != galleries.GallerySharingPermissionTypesCommunity {
			continue
		}

		galleryID, err := commonids.ParseSharedImageGalleryIDInsensitively(*g.Id)
		if err != nil {
			return err
		}

		if !o.ActuallyDelete {
			log.Printf("[INFO] would have deleted %s", galleryID)
		}

		// Ensure gallery is not shared as this prevents deletion
		payload := gallerysharingupdate.SharingUpdate{
			OperationType: gallerysharingupdate.SharingUpdateOperationTypesReset,
		}
		if err := computeClient.GallerySharingUpdate.GallerySharingProfileUpdateThenPoll(ctx, *galleryID, payload); err != nil {
			return fmt.Errorf("resetting sharing profile for %s: %w", id, err)
		}

		if _, err := computeClient.Galleries.Delete(ctx, *galleryID); err != nil {
			return fmt.Errorf("deleting %s: %w", id, err)
		}
	}

	return nil
}

func (c computeGalleryCleaner) ResourceTypes() []string {
	return []string{
		"Microsoft.Compute/galleries",
	}
}
