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
	modelv1alpha1 "github.com/vllm-project/aibrix/api/model/v1alpha1"
	"github.com/vllm-project/aibrix/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func buildModelAdapterEndpointSlice(instance *modelv1alpha1.ModelAdapter, pods []corev1.Pod) *discoveryv1.EndpointSlice {
	serviceLabels := map[string]string{
		"kubernetes.io/service-name": instance.Name,
	}

	// Propagate tenant label if present
	if tenantID, ok := instance.Labels[constants.TenantLabelID]; ok {
		serviceLabels[constants.TenantLabelID] = tenantID
	}

	addresses := make([]discoveryv1.Endpoint, 0, len(pods))
	for _, pod := range pods {
		addresses = append(addresses, discoveryv1.Endpoint{
			Addresses: []string{pod.Status.PodIP},
		})
	}

	ports := []discoveryv1.EndpointPort{
		{
			Name:     ptr.To("http"),
			Protocol: ptr.To(corev1.ProtocolTCP),
			Port:     ptr.To(int32(8000)),
		},
	}

	return &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        instance.Name,
			Namespace:   instance.Namespace,
			Labels:      serviceLabels,
			Annotations: make(map[string]string),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(instance, controllerKind),
			},
		},
		AddressType: discoveryv1.AddressTypeIPv4,
		Endpoints:   addresses,
		Ports:       ports,
	}
}

func buildModelAdapterService(instance *modelv1alpha1.ModelAdapter) *corev1.Service {
	labels := map[string]string{
		ModelAdapterKey: instance.Name,
	}
	if instance.Spec.BaseModel != nil {
		labels[ModelIdentifierKey] = *instance.Spec.BaseModel
	}

	// Propagate tenant label if present
	if tenantID, ok := instance.Labels[constants.TenantLabelID]; ok {
		labels[constants.TenantLabelID] = tenantID
	}

	ports := []corev1.ServicePort{
		{
			Name: "http",
			// it should use the base model service port.
			// make sure this can be dynamically configured later.
			Port: 8000,
			TargetPort: intstr.IntOrString{
				Type:   intstr.Int,
				IntVal: 8000,
			},
			Protocol: corev1.ProtocolTCP,
		},
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        instance.Name,
			Namespace:   instance.Namespace,
			Labels:      labels,
			Annotations: make(map[string]string),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(instance, controllerKind),
			},
		},
		Spec: corev1.ServiceSpec{
			ClusterIP:                corev1.ClusterIPNone,
			PublishNotReadyAddresses: true,
			Ports:                    ports,
		},
	}
}

func buildHTTPRoute(instance *modelv1alpha1.ModelAdapter) *gatewayv1.HTTPRoute {
	labels := map[string]string{
		ModelAdapterKey: instance.Name,
	}
	if instance.Spec.BaseModel != nil {
		labels[ModelIdentifierKey] = *instance.Spec.BaseModel
	}

	// Propagate tenant label if present
	if tenantID, ok := instance.Labels[constants.TenantLabelID]; ok {
		labels[constants.TenantLabelID] = tenantID
	}

	// Define ParentRef (Gateway)
	// TODO: Make this configurable via flags or config
	gatewayNamespace := gatewayv1.Namespace("aibrix-system")
	gatewayName := gatewayv1.ObjectName("aibrix-gateway")
	parentRefs := []gatewayv1.ParentReference{
		{
			Group:     ptr.To(gatewayv1.Group("gateway.networking.k8s.io")),
			Kind:      ptr.To(gatewayv1.Kind("Gateway")),
			Namespace: &gatewayNamespace,
			Name:      gatewayName,
		},
	}

	// Define Match
	matches := []gatewayv1.HTTPRouteMatch{}

	// Match on Model ID header
	// The client is expected to send x-aibrix-model-id header to route to the specific adapter
	modelHeaderName := gatewayv1.HTTPHeaderName("x-aibrix-model-id")
	modelHeaderValue := instance.Name
	matches = append(matches, gatewayv1.HTTPRouteMatch{
		Headers: []gatewayv1.HTTPHeaderMatch{
			{
				Name:  modelHeaderName,
				Value: modelHeaderValue,
				Type:  ptr.To(gatewayv1.HeaderMatchExact),
			},
		},
	})

	// If tenant label exists, add tenant header match
	if tenantID, ok := instance.Labels[constants.TenantLabelID]; ok {
		tenantHeaderName := gatewayv1.HTTPHeaderName("x-aibrix-tenant-id")
		matches[0].Headers = append(matches[0].Headers, gatewayv1.HTTPHeaderMatch{
			Name:  tenantHeaderName,
			Value: tenantID,
			Type:  ptr.To(gatewayv1.HeaderMatchExact),
		})
	}

	// BackendRef
	backendRefs := []gatewayv1.HTTPBackendRef{
		{
			BackendRef: gatewayv1.BackendRef{
				BackendObjectReference: gatewayv1.BackendObjectReference{
					Name: gatewayv1.ObjectName(instance.Name),
					Port: ptr.To(gatewayv1.PortNumber(8000)), // Service port
				},
			},
		},
	}

	return &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(instance, controllerKind),
			},
		},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: parentRefs,
			},
			Rules: []gatewayv1.HTTPRouteRule{
				{
					Matches:     matches,
					BackendRefs: backendRefs,
				},
			},
		},
	}
}
