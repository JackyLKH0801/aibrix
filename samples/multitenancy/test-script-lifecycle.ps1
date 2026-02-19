$url = "http://127.0.0.1:8095/v1/completions"
$baseModel = "qwen2.5-1.5b-instruct"

function Send-Request {
    param($tid, $model)
    $headers = @{ "Content-Type"="application/json"; "X-Tenant-ID"=$tid; "model"=$model }
    $body = @{ model=$baseModel; prompt="Lifecycle Test check"; max_tokens=5 } | ConvertTo-Json
    try {
        $resp = Invoke-RestMethod -Uri $url -Method Post -Headers $headers -Body $body -TimeoutSec 5
        return $true
    } catch {
        return $false
    }
}

Write-Host "`n=== Phase 3: Lifecycle & Resilience Test ==="
Write-Host "Initial Check: Both Tenants should be UP"
$a = Send-Request "tenant-a" "qwen-code-lora-tenant"
$b = Send-Request "tenant-b" "qwen-code-lora-tenant-b"
if ($a -and $b) { Write-Host "Verified: Both tenants active." -ForegroundColor Green }
else { Write-Host "Startup check failed." -ForegroundColor Red; exit 1 }

Write-Host "`n[Action] Deleting Tenant B ModelAdapter..."
kubectl delete modeladapter qwen-code-lora-tenant-b --wait=$false
Write-Host "Check: Does Tenant A still work immediately?"

$stillWorks = Send-Request "tenant-a" "qwen-code-lora-tenant"
if ($stillWorks) { Write-Host "SUCCESS: Tenant A is unaffected by Tenant B deletion." -ForegroundColor Green }
else { Write-Host "FAILURE: Tenant A stopped working!" -ForegroundColor Red }

Write-Host "Waiting for deletion to finalize..."
Start-Sleep -Seconds 10

Write-Host "Check: Tenant B should be GONE (404/503)"
$b_gone = Send-Request "tenant-b" "qwen-code-lora-tenant-b"
if (-not $b_gone) { Write-Host "SUCCESS: Tenant B Access Denied." -ForegroundColor Green }
else { Write-Host "FAILURE: Tenant B is still accessible!" -ForegroundColor Red }

Write-Host "`n[Action] Restoring Tenant B..."
kubectl apply -f tenant-b.yaml
Write-Host "Waiting for recovery (15s)..."
Start-Sleep -Seconds 15

# Note: Automatic route regeneration might fail due to the controller issue we found earlier.
# This test also verifies if the manual patch is lost (it will be).
Write-Host "Check: Is Tenant B back online?"
$b_back = Send-Request "tenant-b" "qwen-code-lora-tenant-b"

if ($b_back) { 
    Write-Host "SUCCESS: Tenant B restored automatically." -ForegroundColor Green 
} else {
    Write-Host "OBSERVATION: Tenant B restored but access denied." -ForegroundColor Yellow
    Write-Host "  (Likely requires manual HTTPRoute patch again, confirming controller behavior)"
}

