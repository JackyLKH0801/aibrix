/*
Copyright 2025 The Aibrix Team.

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
	"context"
	"testing"

	configPb "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extProcPb "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/vllm-project/aibrix/pkg/utils"
	v1 "k8s.io/api/core/v1"
)

func TestMultiTenantRequestFlow(t *testing.T) {
	// Setup
	mockCache := new(MockCache)
	mockGatewayClient := new(MockGatewayClient)
	
	server := &Server{
		cache:         mockCache,
		gatewayClient: mockGatewayClient,
		config: GatewayConfig{
			EnforceAuth: false,
		},
		authConfig: AuthConfig{
			EnforceAuth: false,
		},
	}

	// Reset metrics
	requestTotal.Reset()
	registerTenantMetrics()

	tests := []struct {
		name           string
		headers        []*configPb.HeaderValue
		body           string
		expectedTenant string
		expectedModel  string
		expectError    bool
	}{
		{
			name: "Valid Tenant Header",
			headers: []*configPb.HeaderValue{
				{Key: "X-Tenant-ID", RawValue: []byte("tenant-A")},
				{Key: ":path", RawValue: []byte("/v1/completions")},
				{Key: "routing-strategy", RawValue: []byte("random")},
			},
			body:           `{"model": "llama-2-7b"}`,
			expectedTenant: "tenant-A",
			expectedModel:  "llama-2-7b",
			expectError:    false,
		},
		{
			name: "Default Tenant (No Header)",
			headers: []*configPb.HeaderValue{
				{Key: ":path", RawValue: []byte("/v1/completions")},
				{Key: "routing-strategy", RawValue: []byte("random")},
			},
			body:           `{"model": "llama-2-7b"}`,
			expectedTenant: "default",
			expectedModel:  "llama-2-7b",
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock expectations
			mockCache.On("HasModel", tt.expectedModel).Return(true)
			mockCache.On("ListPodsByModel", tt.expectedModel, tt.expectedTenant).Return(&utils.PodArray{
				Pods: []*v1.Pod{
					{
						Status: v1.PodStatus{
							PodIP: "10.0.0.1",
							Conditions: []v1.PodCondition{
								{Type: v1.PodReady, Status: v1.ConditionTrue},
							},
						},
					},
				},
			}, nil)
			mockCache.On("AddRequestCount", mock.Anything, mock.Anything, tt.expectedModel).Return(int64(1))

			// 1. Handle Request Headers
			reqHeaders := &extProcPb.ProcessingRequest{
				Request: &extProcPb.ProcessingRequest_RequestHeaders{
					RequestHeaders: &extProcPb.HttpHeaders{
						Headers: &configPb.HeaderMap{
							Headers: tt.headers,
						},
					},
				},
			}

			_, _, _, routingCtx := server.HandleRequestHeaders(context.Background(), "req-id", reqHeaders)
			
			if tt.expectError {
				assert.Nil(t, routingCtx)
				return
			}
			
			assert.NotNil(t, routingCtx)
			assert.Equal(t, tt.expectedTenant, routingCtx.TenantID)

			// 2. Handle Request Body
			reqBody := &extProcPb.ProcessingRequest{
				Request: &extProcPb.ProcessingRequest_RequestBody{
					RequestBody: &extProcPb.HttpBody{
						Body: []byte(tt.body),
					},
				},
			}

			// We need to pass the routingCtx from headers to body
			_, model, _, _, _ := server.HandleRequestBody(routingCtx, "req-id", reqBody, utils.User{})

			assert.Equal(t, tt.expectedModel, model)

			// 3. Verify Metrics
			// We check if the metric with specific labels has value 1
			// Note: Since we run multiple tests, the counter might accumulate if we don't reset.
			// But we reset at the beginning. For subsequent tests, we might need to check for increase.
			// However, since we use different tenants/models or just check existence, let's check value.
			
			val := testutil.ToFloat64(requestTotal.With(prometheus.Labels{
				"tenant_id": tt.expectedTenant,
				"model_id":  tt.expectedModel,
			}))
			assert.GreaterOrEqual(t, val, 1.0, "Metric should be recorded")
		})
	}
}
