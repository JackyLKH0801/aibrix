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
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	configPb "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"k8s.io/klog/v2"
)

const (
	// JWT claim names for tenant metadata
	jwtClaimTenantID     = "tenant_id"
	jwtClaimDeploymentID = "deployment_id"
	jwtClaimPriority     = "priority"

	// Cache key separator
	cacheKeySeparator = "::"
)

// TenantMetadata represents tenant-specific metadata
type TenantMetadata struct {
	TenantID       string
	DeploymentID   string
	DeploymentRev  string
	TenantMetadata string // JSON blob for LoRA/auth context

	// Source-tracking fields used for conflict validation
	HeaderTenantID     string
	HeaderDeploymentID string
	ClaimTenantID      string
	ClaimDeploymentID  string
}

// extractTenantMetadata extracts tenant information from request headers and JWT claims
// Headers take precedence over JWT claims unless policy forbids it
func extractTenantMetadata(headers []*configPb.HeaderValue, authHeader string, verifiedClaims map[string]interface{}) (*TenantMetadata, error) {
	metadata := &TenantMetadata{}

	// Extract from headers first (higher precedence)
	for _, header := range headers {
		key := strings.ToLower(header.Key)
		value := string(header.RawValue)

		switch key {
		case HeaderTenantID:
			metadata.HeaderTenantID = value
			metadata.TenantID = value
		case HeaderDeploymentID:
			metadata.HeaderDeploymentID = value
			metadata.DeploymentID = value
		case HeaderTenantMetadata:
			metadata.TenantMetadata = value
		}
	}

	// Extract from JWT claims if headers not present
	var claims map[string]interface{}
	var err error

	if verifiedClaims != nil {
		claims = verifiedClaims
	} else if authHeader != "" {
		claims, err = extractJWTClaims(authHeader)
		if err != nil {
			klog.V(4).InfoS("Failed to extract JWT claims", "error", err)
		}
	}

	if claims != nil {
		if tenantID, ok := claims[jwtClaimTenantID].(string); ok && tenantID != "" {
			metadata.ClaimTenantID = tenantID
			if metadata.TenantID == "" {
				metadata.TenantID = tenantID
			}
		}
		if deploymentID, ok := claims[jwtClaimDeploymentID].(string); ok && deploymentID != "" {
			metadata.ClaimDeploymentID = deploymentID
			if metadata.DeploymentID == "" {
				metadata.DeploymentID = deploymentID
			}
		}
	}

	// Apply default tenant for legacy requests (compatibility shim)
	if metadata.TenantID == "" {
		metadata.TenantID = DefaultTenant
		recordLegacyTenantFallback()
		klog.Warning("No tenant ID provided, using default tenant for backward compatibility")
	}

	return metadata, nil
}

// extractJWTClaims extracts claims from a JWT token without verification
// This is a simplified implementation - in production, proper JWT validation should be used
func extractJWTClaims(authHeader string) (map[string]interface{}, error) {
	// Extract token from "Bearer <token>" format
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return nil, fmt.Errorf("invalid authorization header format")
	}

	token := parts[1]

	// JWT format: header.payload.signature
	tokenParts := strings.Split(token, ".")
	if len(tokenParts) != 3 {
		return nil, fmt.Errorf("invalid JWT token format")
	}

	// Decode payload (second part)
	payload, err := base64.RawURLEncoding.DecodeString(tokenParts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode JWT payload: %w", err)
	}

	// Parse JSON claims
	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse JWT claims: %w", err)
	}

	return claims, nil
}

// BuildCompositeKey creates a composite cache key in the format: <tenant_id>::<model_id>::<deployment_rev>
// Exported for use by cache layer and routing logic
func BuildCompositeKey(tenantID, modelID, deploymentRev string) string {
	if deploymentRev == "" {
		// Generate a simple hash if deployment revision is not provided
		deploymentRev = "default"
	}
	return fmt.Sprintf("%s%s%s%s%s", tenantID, cacheKeySeparator, modelID, cacheKeySeparator, deploymentRev)
}

// ParseCompositeKey parses a composite cache key back into its components
// Exported for use by cache layer and routing logic
func ParseCompositeKey(key string) (tenantID, modelID, deploymentRev string, err error) {
	parts := strings.Split(key, cacheKeySeparator)
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("invalid composite key format: %s", key)
	}
	return parts[0], parts[1], parts[2], nil
}

// GenerateDeploymentRev generates a deployment revision hash from spec and adapter info
// This ensures cache key uniqueness after rollouts
// Exported for use by controllers
func GenerateDeploymentRev(spec, adapters string) string {
	combined := fmt.Sprintf("%s|%s", spec, adapters)
	hash := sha256.Sum256([]byte(combined))
	return fmt.Sprintf("%x", hash[:8]) // Use first 8 bytes of hash
}

// validateTenantMetadata validates tenant metadata and returns error if invalid
func validateTenantMetadata(metadata *TenantMetadata) error {
	// Tenant ID is required (except for default tenant)
	if metadata.TenantID == "" {
		return fmt.Errorf("tenant ID cannot be empty")
	}

	// Validate tenant ID format (alphanumeric, hyphens, underscores)
	if !isValidTenantID(metadata.TenantID) {
		return fmt.Errorf("invalid tenant ID format: %s", metadata.TenantID)
	}

	// Validate JSON metadata if provided
	if metadata.TenantMetadata != "" {
		var jsonTest interface{}
		if err := json.Unmarshal([]byte(metadata.TenantMetadata), &jsonTest); err != nil {
			return fmt.Errorf("invalid tenant metadata JSON: %w", err)
		}
	}

	return nil
}

// detectTenantConflicts ensures header and claim identifiers do not disagree
func detectTenantConflicts(metadata *TenantMetadata) error {
	if metadata == nil {
		return nil
	}

	if metadata.HeaderTenantID != "" && metadata.ClaimTenantID != "" && metadata.HeaderTenantID != metadata.ClaimTenantID {
		recordTenantConflict("tenant_id")
		return fmt.Errorf("conflicting tenant identifiers: header=%s, claim=%s", metadata.HeaderTenantID, metadata.ClaimTenantID)
	}

	if metadata.HeaderDeploymentID != "" && metadata.ClaimDeploymentID != "" && metadata.HeaderDeploymentID != metadata.ClaimDeploymentID {
		recordTenantConflict("deployment_id")
		return fmt.Errorf("conflicting deployment identifiers: header=%s, claim=%s", metadata.HeaderDeploymentID, metadata.ClaimDeploymentID)
	}

	return nil
}

// isValidTenantID checks if tenant ID contains only allowed characters
func isValidTenantID(tenantID string) bool {
	if tenantID == "" {
		return false
	}

	for _, ch := range tenantID {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '-' || ch == '_') {
			return false
		}
	}
	return true
}
