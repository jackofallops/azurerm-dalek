package dalek

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/go-azure-helpers/lang/pointer"
	"github.com/hashicorp/go-azure-sdk/microsoft-graph/applications/stable/application"
	"github.com/hashicorp/go-azure-sdk/microsoft-graph/common-types/stable"
	"github.com/hashicorp/go-azure-sdk/microsoft-graph/directory/stable/administrativeunit"
	"github.com/hashicorp/go-azure-sdk/microsoft-graph/directory/stable/deleteditem"
	"github.com/hashicorp/go-azure-sdk/microsoft-graph/groups/stable/group"
	"github.com/hashicorp/go-azure-sdk/microsoft-graph/serviceprincipals/stable/serviceprincipal"
	"github.com/hashicorp/go-azure-sdk/microsoft-graph/users/stable/user"
	"github.com/hashicorp/go-azure-sdk/sdk/odata"
)

func (d *Dalek) MicrosoftGraph(ctx context.Context) error {
	//log.Printf("[DEBUG] Preparing to delete Service Principals")
	//if err := d.deleteMicrosoftGraphServicePrincipals(ctx); err != nil {
	//	return fmt.Errorf("deleting Service Principals: %+v", err)
	//}
	//
	//log.Printf("[DEBUG] Preparing to delete Applications")
	//if err := d.deleteMicrosoftGraphApplications(ctx); err != nil {
	//	return fmt.Errorf("deleting Applications: %+v", err)
	//}
	//
	//log.Printf("[DEBUG] Preparing to delete Groups")
	//if err := d.deleteMicrosoftGraphGroups(ctx); err != nil {
	//	return fmt.Errorf("deleting Groups: %+v", err)
	//}
	//
	//log.Printf("[DEBUG] Preparing to delete Users")
	//if err := d.deleteMicrosoftGraphUsers(ctx); err != nil {
	//	return fmt.Errorf("deleting Users: %+v", err)
	//}

	log.Printf("[DEBUG] Preparing to delete Administrative Units")
	if err := d.deleteMicrosoftGraphAdministrativeUnits(ctx); err != nil {
		return fmt.Errorf("deleting Administrative Units: %+v", err)
	}

	return nil
}

func (d *Dalek) deleteMicrosoftGraphApplications(ctx context.Context) error {
	if len(d.opts.Prefix) == 0 {
		return fmt.Errorf("[ERROR] Not proceeding to delete Microsoft Graph Applications for safety; prefix not specified")
	}

	client := d.client.MicrosoftGraph.Applications
	deletedItemClient := d.client.MicrosoftGraph.DeletedItems

	listOptions := application.ListApplicationsOperationOptions{
		Filter: pointer.To(fmt.Sprintf("startswith(displayName, '%s')", d.opts.Prefix)),
	}
	resp, err := client.ListApplications(ctx, listOptions)
	if err != nil {
		return fmt.Errorf("listing Microsoft Graph Applications with prefix %q: %+v", d.opts.Prefix, err)
	}
	if resp.Model == nil {
		return nil
	}

	for _, app := range *resp.Model {
		if app.Id == nil {
			continue
		}

		id := *app.Id
		appID := app.AppId.GetOrZero()
		displayName := app.DisplayName.GetOrZero()

		if strings.TrimPrefix(displayName, d.opts.Prefix) != displayName {
			if !d.opts.ActuallyDelete {
				log.Printf("[DEBUG] Would have deleted Microsoft Graph Application %q (AppID: %s, ObjID: %s)", displayName, appID, id)
				continue
			}

			log.Printf("[DEBUG] Deleting Microsoft Graph Application %q (AppID: %s, ObjectId: %s)...", displayName, appID, id)
			if _, err := client.DeleteApplication(ctx, stable.NewApplicationID(id), application.DefaultDeleteApplicationOperationOptions()); err != nil {
				log.Printf("[DEBUG] Error during deletion of Microsoft Graph Application %q (AppID: %s, ObjID: %s): %s", displayName, appID, id, err)
				continue
			}
			log.Printf("[DEBUG] Deleted Microsoft Graph Application %q (AppID: %s, ObjID: %s)", displayName, appID, id)
		}
	}

	deletedListOptions := deleteditem.ListDeletedItemApplicationsOperationOptions{
		Select: pointer.To([]string{"id", "displayName"}),
	}

	deletedResp, err := deletedItemClient.ListDeletedItemApplicationsComplete(ctx, deletedListOptions)
	if err != nil {
		return fmt.Errorf("listing deleted applications: %+v", err)
	}

	for _, g := range deletedResp.Items {
		if g.Id == nil {
			continue
		}

		id := *g.Id
		displayName := g.DisplayName.GetOrZero()

		// TODO: Arguably if an application has been deleted we can quite safely assume it can be purged, remove check?
		if strings.TrimPrefix(displayName, d.opts.Prefix) == displayName {
			continue
		}

		if !d.opts.ActuallyDelete {
			log.Printf("[DEBUG] Would have purged Microsoft Graph Application %q (ObjID: %s)", displayName, id)
			continue
		}

		log.Printf("[DEBUG] Purging Microsoft Graph Application %q (ObjectId: %s)...", displayName, id)
		if _, err := deletedItemClient.DeleteDeletedItem(ctx, stable.NewDirectoryDeletedItemID(id), deleteditem.DefaultDeleteDeletedItemOperationOptions()); err != nil {
			log.Printf("[DEBUG] Error during purging of Microsoft Graph Application %q (ObjID: %s): %s", displayName, id, err)
			continue
		}
		log.Printf("[DEBUG] Purged Microsoft Graph Application %q (ObjID: %s)", displayName, id)
	}

	return nil
}

func (d *Dalek) deleteMicrosoftGraphGroups(ctx context.Context) error {
	if len(d.opts.Prefix) == 0 {
		return fmt.Errorf("[ERROR] Not proceeding to delete Microsoft Graph Groups for safety; prefix not specified")
	}

	client := d.client.MicrosoftGraph.Groups
	deletedItemClient := d.client.MicrosoftGraph.DeletedItems

	listOptions := group.ListGroupsOperationOptions{
		Filter: pointer.To(fmt.Sprintf("startswith(displayName, '%s')", d.opts.Prefix)),
	}
	resp, err := client.ListGroups(ctx, listOptions)
	if err != nil {
		return fmt.Errorf("[ERROR] Unable to list Microsoft Graph Groups with prefix: %q", d.opts.Prefix)
	}
	if resp.Model == nil {
		return nil
	}

	for _, g := range *resp.Model {
		if g.Id == nil {
			continue
		}

		id := *g.Id
		displayName := g.DisplayName.GetOrZero()

		if strings.TrimPrefix(displayName, d.opts.Prefix) != displayName {
			if !d.opts.ActuallyDelete {
				log.Printf("[DEBUG] Would have deleted Microsoft Graph Group %q (ObjID: %s)", displayName, id)
				continue
			}

			log.Printf("[DEBUG] Deleting Microsoft Graph Group %q (ObjectId: %s)...", displayName, id)
			if _, err := client.DeleteGroup(ctx, stable.NewGroupID(id), group.DefaultDeleteGroupOperationOptions()); err != nil {
				log.Printf("[DEBUG] Error during deletion of Microsoft Graph Group %q (ObjID: %s): %s", displayName, id, err)
				continue
			}
			log.Printf("[DEBUG] Deleted Microsoft Graph Group %q (ObjID: %s)", displayName, id)
		}
	}

	deletedListOptions := deleteditem.ListDeletedItemGroupsOperationOptions{
		Select: pointer.To([]string{"id", "displayName"}),
	}

	deletedResp, err := deletedItemClient.ListDeletedItemGroupsComplete(ctx, deletedListOptions)
	if err != nil {
		return fmt.Errorf("listing deleted groups: %+v", err)
	}

	for _, g := range deletedResp.Items {
		if g.Id == nil {
			continue
		}

		id := *g.Id
		displayName := g.DisplayName.GetOrZero()

		// TODO: Arguably if a group has been deleted we can quite safely assume it can be purged, remove check?
		if strings.TrimPrefix(displayName, d.opts.Prefix) == displayName {
			continue
		}

		if !d.opts.ActuallyDelete {
			log.Printf("[DEBUG] Would have purged Microsoft Graph Group %q (ObjID: %s)", displayName, id)
			continue
		}

		log.Printf("[DEBUG] Purging Microsoft Graph Group %q (ObjectId: %s)...", displayName, id)
		if _, err := deletedItemClient.DeleteDeletedItem(ctx, stable.NewDirectoryDeletedItemID(id), deleteditem.DefaultDeleteDeletedItemOperationOptions()); err != nil {
			log.Printf("[DEBUG] Error during purging of Microsoft Graph Group %q (ObjID: %s): %s", displayName, id, err)
			continue
		}
		log.Printf("[DEBUG] Purged Microsoft Graph Group %q (ObjID: %s)", displayName, id)
	}

	return nil
}

func (d *Dalek) deleteMicrosoftGraphServicePrincipals(ctx context.Context) error {
	if len(d.opts.Prefix) == 0 {
		return fmt.Errorf("[ERROR] Not proceeding to delete Microsoft Graph Service Principals for safety; prefix not specified")
	}

	client := d.client.MicrosoftGraph.ServicePrincipals
	deletedItemClient := d.client.MicrosoftGraph.DeletedItems
	//
	listOptions := serviceprincipal.ListServicePrincipalsOperationOptions{
		ConsistencyLevel: pointer.To(odata.ConsistencyLevelEventual),
		Count:            pointer.To(true),
		// skip `ManagedIdentity` types as these cannot be deleted using the API
		Filter: pointer.To(fmt.Sprintf("startswith(displayName, '%s') and servicePrincipalType ne 'ManagedIdentity'", d.opts.Prefix)),
	}
	resp, err := client.ListServicePrincipals(ctx, listOptions)
	if err != nil {
		return fmt.Errorf("listing Microsoft Graph Service Principals with prefix %q: %+v", d.opts.Prefix, err)
	}
	if resp.Model == nil {
		return nil
	}

	for _, servicePrincipal := range *resp.Model {
		if servicePrincipal.Id == nil {
			continue
		}

		id := *servicePrincipal.Id
		displayName := servicePrincipal.DisplayName.GetOrZero()

		if strings.TrimPrefix(displayName, d.opts.Prefix) != displayName {
			if !d.opts.ActuallyDelete {
				log.Printf("[DEBUG] Would have deleted Microsoft Graph Service Principal %q (ObjID: %s)", displayName, id)
				continue
			}

			log.Printf("[DEBUG] Deleting Microsoft Graph Service Principal %q (ObjectId: %s)...", displayName, id)
			if _, err := client.DeleteServicePrincipal(ctx, stable.NewServicePrincipalID(id), serviceprincipal.DefaultDeleteServicePrincipalOperationOptions()); err != nil {
				log.Printf("[DEBUG] Error during deletion of Microsoft Graph Service Principal %q (ObjID: %s): %s", displayName, id, err)
				continue
			}
			log.Printf("[DEBUG] Deleted Microsoft Graph Service Principal %q (ObjID: %s)", displayName, id)
		}
	}

	deletedListOptions := deleteditem.ListDeletedItemServicePrincipalsOperationOptions{
		Select: pointer.To([]string{"id", "displayName", "servicePrincipalType"}),
	}

	deletedResp, err := deletedItemClient.ListDeletedItemServicePrincipalsComplete(ctx, deletedListOptions)
	if err != nil {
		return fmt.Errorf("listing deleted service principals: %+v", err)
	}

	for _, g := range deletedResp.Items {
		if g.Id == nil {
			continue
		}

		// filter this here rather than server side because there is currently no way to pass `ConsistencyLevel` to this List nmethod
		if g.ServicePrincipalType.GetOrZero() == "ManagedIdentity" {
			continue
		}

		id := *g.Id
		displayName := g.DisplayName.GetOrZero()

		// TODO: Arguably if a service principal has been deleted we can quite safely assume it can be purged, remove check?
		if strings.TrimPrefix(displayName, d.opts.Prefix) == displayName {
			continue
		}

		if !d.opts.ActuallyDelete {
			log.Printf("[DEBUG] Would have purged Microsoft Graph Service Principal %q (ObjID: %s)", displayName, id)
			continue
		}

		log.Printf("[DEBUG] Purging Microsoft Graph Service Principal %q (ObjectId: %s)...", displayName, id)
		if _, err := deletedItemClient.DeleteDeletedItem(ctx, stable.NewDirectoryDeletedItemID(id), deleteditem.DefaultDeleteDeletedItemOperationOptions()); err != nil {
			log.Printf("[DEBUG] Error during purging of Microsoft Graph Service Principal %q (ObjID: %s): %s", displayName, id, err)
			return fmt.Errorf("deleting deleted items: %+v", err)
		}
		log.Printf("[DEBUG] Purged Microsoft Graph Service Principal %q (ObjID: %s)", displayName, id)
	}

	return nil
}

func (d *Dalek) deleteMicrosoftGraphUsers(ctx context.Context) error {
	if len(d.opts.Prefix) == 0 {
		return fmt.Errorf("[ERROR] Not proceeding to delete Microsoft Graph Users for safety; prefix not specified")
	}

	client := d.client.MicrosoftGraph.Users
	deletedItemClient := d.client.MicrosoftGraph.DeletedItems

	listOptions := user.ListUsersOperationOptions{
		Filter: pointer.To(fmt.Sprintf("startswith(displayName, '%s')", d.opts.Prefix)),
	}
	resp, err := client.ListUsers(ctx, listOptions)
	if err != nil {
		return fmt.Errorf("[ERROR] Unable to list Microsoft Graph Users with prefix: %q", d.opts.Prefix)
	}
	if resp.Model == nil {
		return nil
	}

	for _, u := range *resp.Model {
		if u.Id == nil {
			continue
		}

		id := *u.Id
		displayName := u.DisplayName.GetOrZero()

		if strings.TrimPrefix(displayName, d.opts.Prefix) != displayName {
			if !d.opts.ActuallyDelete {
				log.Printf("[DEBUG] Would have deleted Microsoft Graph User %q (ObjID: %s)", displayName, id)
				continue
			}

			log.Printf("[DEBUG] Deleting Microsoft Graph User %q (ObjectId: %s)...", displayName, id)
			if _, err := client.DeleteUser(ctx, stable.NewUserID(id), user.DefaultDeleteUserOperationOptions()); err != nil {
				log.Printf("[DEBUG] Error during deletion of Microsoft Graph User %q (ObjID: %s): %s", displayName, id, err)
				continue
			}
			log.Printf("[DEBUG] Deleted Microsoft Graph User %q (ObjID: %s)", displayName, id)
		}
	}

	deletedListOptions := deleteditem.ListDeletedItemUsersOperationOptions{
		Select: pointer.To([]string{"id", "displayName"}),
	}

	deletedResp, err := deletedItemClient.ListDeletedItemUsersComplete(ctx, deletedListOptions)
	if err != nil {
		return fmt.Errorf("listing deleted users: %+v", err)
	}

	for _, g := range deletedResp.Items {
		if g.Id == nil {
			continue
		}

		id := *g.Id
		displayName := g.DisplayName.GetOrZero()

		// TODO: Arguably if a user has been deleted we can quite safely assume it can be purged, remove check?
		if strings.TrimPrefix(displayName, d.opts.Prefix) == displayName {
			continue
		}

		if !d.opts.ActuallyDelete {
			log.Printf("[DEBUG] Would have purged Microsoft Graph User %q (ObjID: %s)", displayName, id)
			continue
		}

		log.Printf("[DEBUG] Purging Microsoft Graph User %q (ObjectId: %s)...", displayName, id)
		if _, err := deletedItemClient.DeleteDeletedItem(ctx, stable.NewDirectoryDeletedItemID(id), deleteditem.DefaultDeleteDeletedItemOperationOptions()); err != nil {
			log.Printf("[DEBUG] Error during purging of Microsoft Graph User %q (ObjID: %s): %s", displayName, id, err)
			continue
		}
		log.Printf("[DEBUG] Purged Microsoft Graph User %q (ObjID: %s)", displayName, id)
	}

	return nil
}

func (d *Dalek) deleteMicrosoftGraphAdministrativeUnits(ctx context.Context) error {
	if len(d.opts.Prefix) == 0 {
		return fmt.Errorf("[ERROR] Not proceeding to delete Microsoft Graph Administrative Units for safety; prefix not specified")
	}

	client := d.client.MicrosoftGraph.AdministrativeUnits
	deletedItemClient := d.client.MicrosoftGraph.DeletedItems

	listOptions := administrativeunit.ListAdministrativeUnitsOperationOptions{
		Filter: pointer.To(fmt.Sprintf("startswith(displayName, '%s')", d.opts.Prefix)),
	}
	resp, err := client.ListAdministrativeUnits(ctx, listOptions)
	if err != nil {
		return fmt.Errorf("listing Microsoft Graph Administrative Units with prefix %q: %+v", d.opts.Prefix, err)
	}
	if resp.Model == nil {
		return nil
	}

	for _, au := range *resp.Model {
		if au.Id == nil {
			continue
		}

		id := *au.Id
		displayName := au.DisplayName.GetOrZero()

		if strings.TrimPrefix(displayName, d.opts.Prefix) != displayName {
			if !d.opts.ActuallyDelete {
				log.Printf("[DEBUG] Would have deleted Microsoft Graph Administrative Unit %q (ObjID: %s)", displayName, id)
				continue
			}

			log.Printf("[DEBUG] Deleting Microsoft Graph Administrative Unit %q (ObjectId: %s)...", displayName, id)
			if _, err := client.DeleteAdministrativeUnit(ctx, stable.NewDirectoryAdministrativeUnitID(id), administrativeunit.DefaultDeleteAdministrativeUnitOperationOptions()); err != nil {
				log.Printf("[DEBUG] Error during deletion of Microsoft Graph Administrative Unit %q (ObjID: %s): %s", displayName, id, err)
				continue
			}
			log.Printf("[DEBUG] Deleted Microsoft Graph Administrative Unit %q (ObjID: %s)", displayName, id)
		}
	}

	deletedListOptions := deleteditem.ListDeletedItemAdministrativeUnitsOperationOptions{
		Select: pointer.To([]string{"id", "displayName"}),
	}

	deletedResp, err := deletedItemClient.ListDeletedItemAdministrativeUnitsComplete(ctx, deletedListOptions)
	if err != nil {
		return fmt.Errorf("listing deleted administrative units: %+v", err)
	}

	for _, au := range deletedResp.Items {
		if au.Id == nil {
			continue
		}

		id := *au.Id
		displayName := au.DisplayName.GetOrZero()

		// TODO: Arguably if an administrative unit has been deleted we can quite safely assume it can be purged, remove check?
		if strings.TrimPrefix(displayName, d.opts.Prefix) == displayName {
			continue
		}

		if !d.opts.ActuallyDelete {
			log.Printf("[DEBUG] Would have purged Microsoft Graph Administrative Unit %q (ObjID: %s)", displayName, id)
			continue
		}

		log.Printf("[DEBUG] Purging Microsoft Graph Administrative Unit %q (ObjectId: %s)...", displayName, id)
		if _, err := deletedItemClient.DeleteDeletedItem(ctx, stable.NewDirectoryDeletedItemID(id), deleteditem.DefaultDeleteDeletedItemOperationOptions()); err != nil {
			log.Printf("[DEBUG] Error during purging of Microsoft Graph Administrative Unit %q (ObjID: %s): %s", displayName, id, err)
			continue
		}
		log.Printf("[DEBUG] Purged Microsoft Graph Administrative Unit %q (ObjID: %s)", displayName, id)
	}

	return nil
}
