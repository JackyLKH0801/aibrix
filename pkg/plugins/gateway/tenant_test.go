/*
Copyright 2024 The Aibrix Team.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package gateway

import (
	"testing"

	configPb "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/stretchr/testify/assert"
)

func TestExtractTenantMetadata(t *testing.T) {
	tests := []struct {
		name       string
		headers    []*configPb.HeaderValue
		authHeader string
		expected   *TenantMetadata
		wantErr    bool
	}{
		{
			name: "tenant from header",
			headers: []*configPb.HeaderValue{
				{Key: HeaderTenantID, RawValue: []byte("tenant-123")},
				{Key: HeaderDeploymentID, RawValue: []byte("deploy-456")},
			},
			authHeader: "",
			expected: &TenantMetadata{
				TenantID:     "tenant-123",
				DeploymentID: "deploy-456",
			},
			wantErr: false,
		},
		{
			name:       "default tenant when no header",
			headers:    []*configPb.HeaderValue{},
			authHeader: "",
			expected: &TenantMetadata{
				TenantID: DefaultTenant,
			},
			wantErr: false,
		},
		{
			name: "tenant metadata JSON",
			headers: []*configPb.HeaderValue{
				{Key: HeaderTenantID, RawValue: []byte("tenant-123")},
				{Key: HeaderTenantMetadata, RawValue: []byte(`{"lora":"adapter1"}`)},
			},
			authHeader: "",
			expected: &TenantMetadata{
				TenantID:       "tenant-123",
				TenantMetadata: `{"lora":"adapter1"}`,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := extractTenantMetadata(tt.headers, tt.authHeader, nil)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected.TenantID, result.TenantID)
				assert.Equal(t, tt.expected.DeploymentID, result.DeploymentID)
				assert.Equal(t, tt.expected.TenantMetadata, result.TenantMetadata)
			}
		})
	}
}

func TestBuildCompositeKey(t *testing.T) {
	tests := []struct {
		name          string
		tenantID      string
		modelID       string
		deploymentRev string
		expected      string
	}{
		{
			name:          "full key",
			tenantID:      "tenant-123",
			modelID:       "gpt-4",
			deploymentRev: "abc123",
			expected:      "tenant-123::gpt-4::abc123",
		},
		{
			name:          "default deployment rev",
			tenantID:      "tenant-123",
			modelID:       "gpt-4",
			deploymentRev: "",
			expected:      "tenant-123::gpt-4::default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildCompositeKey(tt.tenantID, tt.modelID, tt.deploymentRev)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseCompositeKey(t *testing.T) {
	tests := []struct {
		name              string
		key               string
		expectedTenantID  string
		expectedModelID   string
		expectedDeployRev string
		wantErr           bool
	}{
		{
			name:              "valid key",
			key:               "tenant-123::gpt-4::abc123",
			expectedTenantID:  "tenant-123",
			expectedModelID:   "gpt-4",
			expectedDeployRev: "abc123",
			wantErr:           false,
		},
		{
			name:    "invalid key format",
			key:     "invalid-key",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tenantID, modelID, deployRev, err := ParseCompositeKey(tt.key)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedTenantID, tenantID)
				assert.Equal(t, tt.expectedModelID, modelID)
				assert.Equal(t, tt.expectedDeployRev, deployRev)
			}
		})
	}
}

func TestGenerateDeploymentRev(t *testing.T) {
	spec1 := "replica:3,gpu:A100"
	adapters1 := "lora-adapter-1"
	adapters2 := "lora-adapter-2"

	rev1 := GenerateDeploymentRev(spec1, adapters1)
	rev2 := GenerateDeploymentRev(spec1, adapters2)
	rev3 := GenerateDeploymentRev(spec1, adapters1)

	// Same inputs should produce same hash
	assert.Equal(t, rev1, rev3)

	// Different adapters should produce different hash
	assert.NotEqual(t, rev1, rev2)

	// Hash should be 16 characters (8 bytes hex)
	assert.Equal(t, 16, len(rev1))
}

func TestValidateTenantMetadata(t *testing.T) {
	tests := []struct {
		name     string
		metadata *TenantMetadata
		wantErr  bool
	}{
		{
			name: "valid tenant",
			metadata: &TenantMetadata{
				TenantID: "tenant-123",
			},
			wantErr: false,
		},
		{
			name: "valid tenant with metadata",
			metadata: &TenantMetadata{
				TenantID:       "tenant-123",
				TenantMetadata: `{"key":"value"}`,
			},
			wantErr: false,
		},
		{
			name: "invalid JSON metadata",
			metadata: &TenantMetadata{
				TenantID:       "tenant-123",
				TenantMetadata: `{invalid json}`,
			},
			wantErr: true,
		},
		{
			name: "invalid tenant ID characters",
			metadata: &TenantMetadata{
				TenantID: "tenant@123!",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTenantMetadata(tt.metadata)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsValidTenantID(t *testing.T) {
	tests := []struct {
		name     string
		tenantID string
		expected bool
	}{
		{"valid alphanumeric", "tenant123", true},
		{"valid with hyphens", "tenant-123", true},
		{"valid with underscores", "tenant_123", true},
		{"valid mixed", "Tenant-123_ABC", true},
		{"invalid with special chars", "tenant@123", false},
		{"invalid with spaces", "tenant 123", false},
		{"invalid with dots", "tenant.123", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidTenantID(tt.tenantID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDetectTenantConflicts(t *testing.T) {
	tests := []struct {
		name     string
		metadata *TenantMetadata
		wantErr  bool
	}{
		{
			name: "matching sources",
			metadata: &TenantMetadata{
				HeaderTenantID:     "tenant-a",
				ClaimTenantID:      "tenant-a",
				HeaderDeploymentID: "deploy-1",
				ClaimDeploymentID:  "deploy-1",
			},
			wantErr: false,
		},
		{
			name: "conflicting tenant IDs",
			metadata: &TenantMetadata{
				HeaderTenantID: "tenant-a",
				ClaimTenantID:  "tenant-b",
			},
			wantErr: true,
		},
		{
			name: "conflicting deployment IDs",
			metadata: &TenantMetadata{
				HeaderDeploymentID: "deploy-1",
				ClaimDeploymentID:  "deploy-2",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := detectTenantConflicts(tt.metadata)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
