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
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"k8s.io/klog/v2"
)

// AuthConfig holds configuration for authentication
type AuthConfig struct {
	// Map of tenant ID to their secret keys (for HMAC)
	// In a real system, this would be backed by a Vault or K8s Secrets
	TenantKeys map[string]string

	// Global secret for legacy/default tenant or fallback
	DefaultSecret string

	// Whether to enforce authentication
	EnforceAuth bool
}

// ValidateToken validates the JWT token and returns the claims
func (s *Server) ValidateToken(authHeader string) (jwt.MapClaims, error) {
	if authHeader == "" {
		return nil, fmt.Errorf("missing authorization header")
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return nil, fmt.Errorf("invalid authorization header format")
	}
	tokenString := parts[1]

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Validate the alg is what you expect:
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		// Extract tenant_id from claims to find the correct key
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			return nil, fmt.Errorf("invalid claims structure")
		}

		// Try to find a key for the tenant
		if tenantID, ok := claims["tenant_id"].(string); ok {
			if key, ok := s.authConfig.TenantKeys[tenantID]; ok {
				return []byte(key), nil
			}
		}

		// Fallback to default secret
		if s.authConfig.DefaultSecret != "" {
			return []byte(s.authConfig.DefaultSecret), nil
		}

		return nil, fmt.Errorf("no valid key found for token")
	})

	if err != nil {
		klog.V(4).InfoS("Token validation failed", "error", err)
		return nil, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}

// Helper to update tenant metadata from verified claims
func updateMetadataFromClaims(metadata *TenantMetadata, claims jwt.MapClaims) {
	if tenantID, ok := claims["tenant_id"].(string); ok && tenantID != "" {
		metadata.ClaimTenantID = tenantID
		// If header didn't provide tenant ID, use claim
		if metadata.TenantID == "" || metadata.TenantID == DefaultTenant {
			metadata.TenantID = tenantID
		}
	}
	if deploymentID, ok := claims["deployment_id"].(string); ok && deploymentID != "" {
		metadata.ClaimDeploymentID = deploymentID
		if metadata.DeploymentID == "" {
			metadata.DeploymentID = deploymentID
		}
	}
}
