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
	"context"
	"strings"

	"k8s.io/klog/v2"

	configPb "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extProcPb "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	envoyTypePb "github.com/envoyproxy/go-control-plane/envoy/type/v3"

	routing "github.com/vllm-project/aibrix/pkg/plugins/gateway/algorithms"
	"github.com/vllm-project/aibrix/pkg/types"
	"github.com/vllm-project/aibrix/pkg/utils"
)

// the key in request headers
const (
	userKey          = "user"
	pathKey          = ":path"
	authorizationKey = "authorization"
)

func (s *Server) HandleRequestHeaders(ctx context.Context, requestID string, req *extProcPb.ProcessingRequest) (*extProcPb.ProcessingResponse, utils.User, int64, *types.RoutingContext) {
	var username, requestPath string
	var user utils.User
	var rpm int64
	var err error
	var errRes *extProcPb.ProcessingResponse
	var routingCtx *types.RoutingContext

	h := req.Request.(*extProcPb.ProcessingRequest_RequestHeaders)
	reqHeaders := map[string]string{}
	var authHeader string

	for _, n := range h.RequestHeaders.Headers.Headers {
		switch strings.ToLower(n.Key) {
		case userKey:
			username = string(n.RawValue)
		case pathKey:
			requestPath = string(n.RawValue)
		case authorizationKey:
			authHeader = string(n.RawValue)
			reqHeaders[n.Key] = authHeader
		}
	}

	// Auth Validation
	var verifiedClaims map[string]interface{}
	if authHeader != "" {
		claims, err := s.ValidateToken(authHeader)
		if err != nil {
			if s.authConfig.EnforceAuth {
				klog.ErrorS(err, "token validation failed", "requestID", requestID)
				return generateErrorResponse(
					envoyTypePb.StatusCode_Unauthorized,
					[]*configPb.HeaderValueOption{{Header: &configPb.HeaderValue{
						Key: HeaderErrorRequestBodyProcessing, RawValue: []byte("unauthorized"),
					}}}, "unauthorized", "", ""), utils.User{}, rpm, routingCtx
			}
			klog.Warningf("Token validation failed but auth not enforced: %v", err)
		} else {
			verifiedClaims = claims
		}
	} else if s.authConfig.EnforceAuth {
		klog.ErrorS(nil, "missing authorization header", "requestID", requestID)
		return generateErrorResponse(
			envoyTypePb.StatusCode_Unauthorized,
			[]*configPb.HeaderValueOption{{Header: &configPb.HeaderValue{
				Key: HeaderErrorRequestBodyProcessing, RawValue: []byte("unauthorized"),
			}}}, "unauthorized", "", ""), utils.User{}, rpm, routingCtx
	}

	// Extract tenant metadata from headers and JWT claims
	tenantMetadata, err := extractTenantMetadata(h.RequestHeaders.Headers.Headers, authHeader, verifiedClaims)
	if err != nil {
		klog.ErrorS(err, "failed to extract tenant metadata", "requestID", requestID)
		return generateErrorResponse(
			envoyTypePb.StatusCode_BadRequest,
			[]*configPb.HeaderValueOption{{Header: &configPb.HeaderValue{
				Key: HeaderErrorRequestBodyProcessing, RawValue: []byte("invalid tenant metadata"),
			}}}, "invalid tenant metadata", "", ""), utils.User{}, rpm, routingCtx
	}

	if err := detectTenantConflicts(tenantMetadata); err != nil {
		klog.ErrorS(err, "tenant metadata conflict", "requestID", requestID)
		return generateErrorResponse(
			envoyTypePb.StatusCode_BadRequest,
			[]*configPb.HeaderValueOption{{Header: &configPb.HeaderValue{
				Key: HeaderErrorRequestBodyProcessing, RawValue: []byte("conflicting tenant metadata"),
			}}}, err.Error(), "", ""), utils.User{}, rpm, routingCtx
	}

	// Validate tenant metadata
	if err := validateTenantMetadata(tenantMetadata); err != nil {
		klog.ErrorS(err, "invalid tenant metadata", "requestID", requestID)
		return generateErrorResponse(
			envoyTypePb.StatusCode_BadRequest,
			[]*configPb.HeaderValueOption{{Header: &configPb.HeaderValue{
				Key: HeaderErrorRequestBodyProcessing, RawValue: []byte("invalid tenant metadata"),
			}}}, err.Error(), "", ""), utils.User{}, rpm, routingCtx
	}

	routingStrategy, routingStrategyEnabled := getRoutingStrategy(h.RequestHeaders.Headers.Headers)
	routingAlgorithm, ok := routing.Validate(routingStrategy)
	if routingStrategyEnabled && !ok {
		klog.ErrorS(nil, "incorrect routing strategy", "requestID", requestID, "routing-strategy", routingStrategy)
		return generateErrorResponse(
			envoyTypePb.StatusCode_BadRequest,
			[]*configPb.HeaderValueOption{{Header: &configPb.HeaderValue{
				Key: HeaderErrorInvalidRouting, RawValue: []byte(routingStrategy),
			}}}, "incorrect routing strategy", "", "routing-strategy"), utils.User{}, rpm, routingCtx
	}

	if username != "" {
		user, err = utils.GetUser(ctx, utils.User{Name: username}, s.redisClient)
		if err != nil {
			klog.ErrorS(err, "unable to process user info", "requestID", requestID, "username", username)
			return generateErrorResponse(
				envoyTypePb.StatusCode_InternalServerError,
				[]*configPb.HeaderValueOption{{Header: &configPb.HeaderValue{
					Key: HeaderErrorUser, RawValue: []byte("true"),
				}}},
				err.Error(), "", ""), utils.User{}, rpm, routingCtx
		}

		rpm, errRes, err = s.checkLimits(ctx, user, tenantMetadata.TenantID)
		if errRes != nil {
			klog.ErrorS(err, "error on checking limits", "requestID", requestID, "username", username, "tenantID", tenantMetadata.TenantID)
			return errRes, utils.User{}, rpm, routingCtx
		}
	}

	routingCtx = types.NewRoutingContext(ctx, routingAlgorithm, "", "", requestID, user.Name)
	routingCtx.ReqPath = requestPath
	routingCtx.ReqHeaders = reqHeaders

	// Populate tenant metadata in routing context
	routingCtx.TenantID = tenantMetadata.TenantID
	routingCtx.DeploymentID = tenantMetadata.DeploymentID
	routingCtx.TenantMetadata = tenantMetadata.TenantMetadata

	headers := []*configPb.HeaderValueOption{}
	headers = append(headers, &configPb.HeaderValueOption{
		Header: &configPb.HeaderValue{
			Key:      HeaderWentIntoReqHeaders,
			RawValue: []byte("true"),
		},
	})

	switch requestPath {
	case "/v1/image/generations":
		headers = buildEnvoyProxyHeaders(headers, ":path", "/generate")
	case "/v1/video/generations":
		headers = buildEnvoyProxyHeaders(headers, ":path", "/generatevideo")
	default:
		break
	}

	return &extProcPb.ProcessingResponse{
		Response: &extProcPb.ProcessingResponse_RequestHeaders{
			RequestHeaders: &extProcPb.HeadersResponse{
				Response: &extProcPb.CommonResponse{
					HeaderMutation: &extProcPb.HeaderMutation{
						SetHeaders: headers,
					},
					ClearRouteCache: true,
				},
			},
		},
	}, user, rpm, routingCtx
}
