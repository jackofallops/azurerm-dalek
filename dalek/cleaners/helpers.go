package cleaners

import "strings"

// isResourceGroupNotFound checks if an error indicates the resource group no longer exists.
// Several Azure SDK flavours (autorest, hashicorp) wrap errors deeply, so we match on the
// Azure error code in the message string rather than relying on a specific error type.
func isResourceGroupNotFound(err error) bool {
	return strings.Contains(err.Error(), "ResourceGroupNotFound")
}
