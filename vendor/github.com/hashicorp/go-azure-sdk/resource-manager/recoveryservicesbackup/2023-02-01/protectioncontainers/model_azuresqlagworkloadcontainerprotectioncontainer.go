package protectioncontainers

import (
	"encoding/json"
	"fmt"
)

// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License. See NOTICE.txt in the project root for license information.

var _ ProtectionContainer = AzureSQLAGWorkloadContainerProtectionContainer{}

type AzureSQLAGWorkloadContainerProtectionContainer struct {
	ExtendedInfo     *AzureWorkloadContainerExtendedInfo `json:"extendedInfo,omitempty"`
	LastUpdatedTime  *string                             `json:"lastUpdatedTime,omitempty"`
	OperationType    *OperationType                      `json:"operationType,omitempty"`
	SourceResourceId *string                             `json:"sourceResourceId,omitempty"`
	WorkloadType     *WorkloadType                       `json:"workloadType,omitempty"`

	// Fields inherited from ProtectionContainer
	BackupManagementType  *BackupManagementType `json:"backupManagementType,omitempty"`
	FriendlyName          *string               `json:"friendlyName,omitempty"`
	HealthStatus          *string               `json:"healthStatus,omitempty"`
	ProtectableObjectType *string               `json:"protectableObjectType,omitempty"`
	RegistrationStatus    *string               `json:"registrationStatus,omitempty"`
}

var _ json.Marshaler = AzureSQLAGWorkloadContainerProtectionContainer{}

func (s AzureSQLAGWorkloadContainerProtectionContainer) MarshalJSON() ([]byte, error) {
	type wrapper AzureSQLAGWorkloadContainerProtectionContainer
	wrapped := wrapper(s)
	encoded, err := json.Marshal(wrapped)
	if err != nil {
		return nil, fmt.Errorf("marshaling AzureSQLAGWorkloadContainerProtectionContainer: %+v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return nil, fmt.Errorf("unmarshaling AzureSQLAGWorkloadContainerProtectionContainer: %+v", err)
	}
	decoded["containerType"] = "SQLAGWorkLoadContainer"

	encoded, err = json.Marshal(decoded)
	if err != nil {
		return nil, fmt.Errorf("re-marshaling AzureSQLAGWorkloadContainerProtectionContainer: %+v", err)
	}

	return encoded, nil
}
