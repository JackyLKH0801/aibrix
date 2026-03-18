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

package modelrouter

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	orchestrationv1alpha1 "github.com/vllm-project/aibrix/api/orchestration/v1alpha1"
	"github.com/vllm-project/aibrix/pkg/constants"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestAddRouteFromStormService(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(orchestrationv1alpha1.AddToScheme(scheme))
	utilruntime.Must(gatewayv1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	r := &ModelRouter{
		Client: fakeClient,
	}

	stormService := &orchestrationv1alpha1.StormService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-stormservice",
			Namespace: "default",
			Labels: map[string]string{
				constants.ModelLabelName: "test-model",
			},
		},
	}

	// Call the handler directly
	r.addRouteFromStormService(stormService)

	// Verify HTTPRoute creation
	httpRoute := &gatewayv1.HTTPRoute{}
	err := fakeClient.Get(context.TODO(), types.NamespacedName{
		Name:      "test-model-router",
		Namespace: aibrixEnvoyGatewayNamespace,
	}, httpRoute)

	assert.NoError(t, err)
	assert.Equal(t, "test-model-router", httpRoute.Name)
	assert.Equal(t, aibrixEnvoyGatewayNamespace, httpRoute.Namespace)
}

func TestDeleteRouteFromStormService(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(orchestrationv1alpha1.AddToScheme(scheme))
	utilruntime.Must(gatewayv1.AddToScheme(scheme))

	// Pre-create the HTTPRoute
	httpRoute := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-model-router",
			Namespace: aibrixEnvoyGatewayNamespace,
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(httpRoute).Build()

	r := &ModelRouter{
		Client: fakeClient,
	}

	stormService := &orchestrationv1alpha1.StormService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-stormservice",
			Namespace: "default",
			Labels: map[string]string{
				constants.ModelLabelName: "test-model",
			},
		},
	}

	// Call the handler directly
	r.deleteRouteFromStormService(stormService)

	// Verify HTTPRoute deletion
	err := fakeClient.Get(context.TODO(), types.NamespacedName{
		Name:      "test-model-router",
		Namespace: aibrixEnvoyGatewayNamespace,
	}, httpRoute)

	assert.Error(t, err)
	assert.True(t, fmt.Sprintf("%v", err) == "httproutes.gateway.networking.k8s.io \"test-model-router\" not found", "Expected error to be about not found")
}
