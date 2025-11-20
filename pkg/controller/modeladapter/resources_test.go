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

package modeladapter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	modelv1alpha1 "github.com/vllm-project/aibrix/api/model/v1alpha1"
	"github.com/vllm-project/aibrix/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestBuildModelAdapterEndpointSlice(t *testing.T) {
	// Mock input for ModelAdapter
	instance := &modelv1alpha1.ModelAdapter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-instance",
			Namespace: "default",
		},
	}

	// Mock input for Pods
	pod := corev1.Pod{
		Status: corev1.PodStatus{
			PodIP: "192.168.1.1",
		},
	}
	pods := []corev1.Pod{pod}

	// Call the function to test
	endpointSlice := buildModelAdapterEndpointSlice(instance, pods)

	// Check EndpointSlice metadata
	assert.Equal(t, "test-instance", endpointSlice.Name)
	assert.Equal(t, "default", endpointSlice.Namespace)
	assert.Equal(t, map[string]string{
		"kubernetes.io/service-name": "test-instance",
	}, endpointSlice.Labels)

	// Check addresses
	assert.Len(t, endpointSlice.Endpoints, 1)
	assert.Equal(t, "192.168.1.1", endpointSlice.Endpoints[0].Addresses[0])

	// Check ports
	assert.Len(t, endpointSlice.Ports, 1)
	assert.Equal(t, "http", *endpointSlice.Ports[0].Name)
	assert.Equal(t, corev1.ProtocolTCP, *endpointSlice.Ports[0].Protocol)
	assert.Equal(t, int32(8000), *endpointSlice.Ports[0].Port)

	// Check owner references
	assert.Len(t, endpointSlice.OwnerReferences, 1)
	assert.Equal(t, instance.Name, endpointSlice.OwnerReferences[0].Name)
}

func TestBuildModelAdapterService(t *testing.T) {
	// Mock input for ModelAdapter
	instance := &modelv1alpha1.ModelAdapter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-instance",
			Namespace: "default",
		},
		Spec: modelv1alpha1.ModelAdapterSpec{
			BaseModel: ptr.To[string]("test-model"),
		},
	}

	// Call the function to test
	service := buildModelAdapterService(instance)

	// Check Service metadata
	assert.Equal(t, "test-instance", service.Name)
	assert.Equal(t, "default", service.Namespace)
	assert.Equal(t, map[string]string{
		constants.ModelLabelName:       "test-model",
		"adapter.model.aibrix.ai/name": "test-instance",
	}, service.Labels)

	// Check ports
	assert.Len(t, service.Spec.Ports, 1)
	assert.Equal(t, int32(8000), service.Spec.Ports[0].Port)
	assert.Equal(t, intstr.FromInt32(8000), service.Spec.Ports[0].TargetPort)
	assert.Equal(t, corev1.ProtocolTCP, service.Spec.Ports[0].Protocol)

	// Check owner references
	assert.Len(t, service.OwnerReferences, 1)
	assert.Equal(t, instance.Name, service.OwnerReferences[0].Name)
}

func TestBuildHTTPRoute(t *testing.T) {
	// Mock input for ModelAdapter with Tenant ID
	instance := &modelv1alpha1.ModelAdapter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-adapter",
			Namespace: "default",
			Labels: map[string]string{
				constants.TenantLabelID: "tenant-123",
			},
		},
		Spec: modelv1alpha1.ModelAdapterSpec{
			BaseModel: ptr.To[string]("test-model"),
		},
	}

	// Call the function to test
	route := buildHTTPRoute(instance)

	// Check HTTPRoute metadata
	assert.Equal(t, "test-adapter", route.Name)
	assert.Equal(t, "default", route.Namespace)
	assert.Equal(t, "tenant-123", route.Labels[constants.TenantLabelID])

	// Check ParentRefs
	assert.Len(t, route.Spec.ParentRefs, 1)
	assert.Equal(t, "aibrix-gateway", string(route.Spec.ParentRefs[0].Name))

	// Check Rules
	assert.Len(t, route.Spec.Rules, 1)
	rule := route.Spec.Rules[0]

	// Check Matches
	assert.Len(t, rule.Matches, 1)
	match := rule.Matches[0]
	assert.Len(t, match.Headers, 2)

	// Check Model ID Header
	assert.Equal(t, gatewayv1.HTTPHeaderName("x-aibrix-model-id"), match.Headers[0].Name)
	assert.Equal(t, "test-adapter", match.Headers[0].Value)

	// Check Tenant ID Header
	assert.Equal(t, gatewayv1.HTTPHeaderName("x-aibrix-tenant-id"), match.Headers[1].Name)
	assert.Equal(t, "tenant-123", match.Headers[1].Value)

	// Check BackendRefs
	assert.Len(t, rule.BackendRefs, 1)
	assert.Equal(t, "test-adapter", string(rule.BackendRefs[0].Name))
	assert.Equal(t, int32(8000), int32(*rule.BackendRefs[0].Port))
}
