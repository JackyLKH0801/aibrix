# Test Script for StormService Routing on Windows
# Run this from workspace root: .\samples\stormservice\test-routing.ps1

$ErrorActionPreference = "Stop"

# Use correct path
$manifestPath = "samples\stormservice\mock-storm-model.yaml"

Write-Host "1. Applying Mock StormService..." -ForegroundColor Cyan
if (Test-Path $manifestPath) {
    kubectl apply -f $manifestPath
} else {
    Write-Error "Manifest file not found at $manifestPath"
}

Write-Host "`n2. Waiting for StormService Pod..." -ForegroundColor Cyan
$start = Get-Date

do {
    $podNames = kubectl get pods -l storm-service-name=mock-storm-model --field-selector=status.phase=Running -o jsonpath="{.items[*].metadata.name}" 
    if ($podNames) {
        $podName = $podNames.Split(" ")[0]
        break
    }
    Start-Sleep -Seconds 2
    Write-Host "." -NoNewline
} while ((Get-Date) -lt $start.AddSeconds(60))
Write-Host ""

if (-not $podName) {
    Write-Warning "StormService pod not yet Running. It might be pulling images or pending."
} else {
    Write-Host "Pod '$podName' is running." -ForegroundColor Green
}

Write-Host "`n3. Verifying HTTPRoute 'mock-storm-model-router'..." -ForegroundColor Cyan
try {
    # Check if the HTTPRoute exists
    $routeJson = kubectl get httproute -n aibrix-system mock-storm-model-router -o json 2>$null | ConvertFrom-Json
    if ($routeJson) {
        Write-Host "SUCCESS: HTTPRoute 'mock-storm-model-router' created!" -ForegroundColor Green
        
        # Check backendRefs
        $backendRef = $routeJson.spec.rules[0].backendRefs[0].backendRef
        if ($backendRef.name -eq "mock-storm-model") {
             Write-Host "Success: BackendRef correctly points to 'mock-storm-model' service/endpoint." -ForegroundColor Green
        } else {
             Write-Warning "Route found, but BackendRef name '$($backendRef.name)' might be incorrect."
        }
    }
} catch {
    Write-Warning "HTTPRoute 'mock-storm-model-router' not found in namespace aibrix-system."
    Write-Host "Troubleshooting:"
    Write-Host "  1. Check controller logs: kubectl logs -n aibrix-system deployment/aibrix-controller-manager | ForEach-Object { if (`$_ -match 'stormservice') { Write-Host `$_ } }"
    Write-Host "  2. Ensure the label 'model.aibrix.ai/name' is present on the StormService."
}

# 4. Gateway Info
Write-Host "`n4. Test Connection Info" -ForegroundColor Cyan
$gwLines = kubectl get svc -n envoy-gateway-system -o name
$gwSvc = $gwLines | Select-string "envoy-aibrix-system-aibrix-eg" | Select-Object -First 1

if ($gwSvc) {
    # Extract only the service name (remove 'service/')
    $svcName = ($gwSvc.ToString() -split "/")[1]
    
    Write-Host "Envoy Gateway Service: $svcName"
    
    Write-Host "`nTo test the routing, execute the following commands in a SEPARATE terminal:" -ForegroundColor Yellow
    Write-Host "kubectl port-forward -n envoy-gateway-system svc/$svcName 8888:80" -ForegroundColor White
    
    Write-Host "`nThen verify the response:" -ForegroundColor Yellow
    Write-Host "curl -v -H 'model: mock-storm-model' http://localhost:8888/v1/chat/completions" -ForegroundColor White
} else {
    Write-Warning "Could not automatically detect the Envoy Gateway service name. Check: kubectl get svc -n envoy-gateway-system"
}
