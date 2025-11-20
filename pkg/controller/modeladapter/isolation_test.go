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

package modeladapter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	modelv1alpha1 "github.com/vllm-project/aibrix/api/model/v1alpha1"
	"github.com/vllm-project/aibrix/pkg/config"
	"github.com/vllm-project/aibrix/pkg/constants"
)

func TestTenantIsolation(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = modelv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	recorder := record.NewFakeRecorder(100)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	reconciler := &ModelAdapterReconciler{
		Client:        client,
		Scheme:        scheme,
		Recorder:      recorder,
		RuntimeConfig: config.RuntimeConfig{},
	}

	ctx := context.Background()

	tests := []struct {
		name            string
		adapterTenant   string
		podTenant       string
		expectedSkip    bool // If true, we expect false, false, nil (skipped)
		expectedProceed bool // If true, we expect it to try loading (and fail due to network/mock)
	}{
		{
			name:            "Matching Tenant IDs",
			adapterTenant:   "tenant-A",
			podTenant:       "tenant-A",
			expectedSkip:    false,
			expectedProceed: true,
		},
		{
			name:            "Mismatching Tenant IDs",
			adapterTenant:   "tenant-A",
			podTenant:       "tenant-B",
			expectedSkip:    true,
			expectedProceed: false,
		},
		{
			name:            "Adapter has Tenant, Pod has no Tenant",
			adapterTenant:   "tenant-A",
			podTenant:       "",
			expectedSkip:    false,
			expectedProceed: true,
		},
		{
			name:            "Adapter has no Tenant, Pod has Tenant",
			adapterTenant:   "",
			podTenant:       "tenant-A",
			expectedSkip:    false,
			expectedProceed: true,
		},
		{
			name:            "Neither has Tenant",
			adapterTenant:   "",
			podTenant:       "",
			expectedSkip:    false,
			expectedProceed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := &modelv1alpha1.ModelAdapter{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-adapter",
					Namespace: "default",
					Labels:    make(map[string]string),
				},
			}
			if tt.adapterTenant != "" {
				adapter.Labels[constants.TenantLabelID] = tt.adapterTenant
			}

			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					Labels:    make(map[string]string),
				},
				Status: corev1.PodStatus{
					PodIP: "10.0.0.1",
				},
			}
			if tt.podTenant != "" {
				pod.Labels[constants.TenantLabelID] = tt.podTenant
			}

			success, shouldRetry, err := reconciler.tryLoadModelAdapterOnPod(ctx, adapter, pod)

			if tt.expectedSkip {
				assert.False(t, success, "Should not succeed")
				assert.False(t, shouldRetry, "Should not retry")
				assert.NoError(t, err, "Should not return error")
			} else if tt.expectedProceed {
				// Since we didn't mock the HTTP calls, it will fail with connection error
				// But the fact that it returns an error means it PASSED the isolation check
				// If it was skipped, it would return nil error
				assert.Error(t, err, "Should return error (connection refused) because it proceeded to load")
			}
		})
	}
}
