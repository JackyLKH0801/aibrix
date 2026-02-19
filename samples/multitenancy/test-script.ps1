$url = "http://127.0.0.1:8095/v1/completions"
$baseModel = "qwen2.5-1.5b-instruct"

# Helper to send request with specific tenant and model
function Test-Tenant {
    param($name, $tid, $routeModelName, $expectSuccess=$true, $missingHeader=$false)
    Write-Host "`n[TEST] Tenant: $name ($tid)"
    if ($missingHeader) {
        Write-Host "       Routing via Header: model=$routeModelName (MISSING X-Tenant-ID)"
    } else {
        Write-Host "       Routing via Header: model=$routeModelName"
    }
    
    $headers = @{ 
        "Content-Type" = "application/json"; 
        # Envoy routes key off this header:
        "model" = $routeModelName
    }
    
    if (-not $missingHeader) {
        $headers["X-Tenant-ID"] = $tid
    }

    # vLLM/Runtime needs to know the ACTUAL running model name
    $body = @{ model = $baseModel; prompt = "Hello from $name"; max_tokens = 20 } | ConvertTo-Json
    
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    try {
        $resp = Invoke-RestMethod -Uri $url -Method Post -Headers $headers -Body $body -TimeoutSec 10
        $sw.Stop()
        if ($expectSuccess) { 
            Write-Host "Response: Success ($($sw.ElapsedMilliseconds)ms)" 
        }
        else { Write-Host "WARNING: Unexpected Success! Request should have failed." }
    } catch {
        $sw.Stop()
        if (-not $expectSuccess) {
             Write-Host "SUCCESS: Access Denied as expected. ($($sw.ElapsedMilliseconds)ms)"
             Write-Host "       (Error: $($_.Exception.Message))"
        }
        else {
             Write-Host "Error: $($_.Exception.Message)"
        }
    }
}

Write-Host "=== Phase 1: Functional Isolation Tests ==="
# 1. Tenant A (Should Succeed)
Test-Tenant -name "Tenant A" -tid "tenant-a" -routeModelName "qwen-code-lora-tenant" -expectSuccess $true

# 2. Tenant B (Should Succeed)
Test-Tenant -name "Tenant B" -tid "tenant-b" -routeModelName "qwen-code-lora-tenant-b" -expectSuccess $true

# 3. Hacker (Should Fail - Wrong Tenant ID)
Test-Tenant -name "Hacker" -tid "hacker-tenant" -routeModelName "qwen-code-lora-tenant" -expectSuccess $false

# 4. Missing Header (Should Fail)
Test-Tenant -name "Missing Header" -tid "none" -routeModelName "qwen-code-lora-tenant" -expectSuccess $false -missingHeader $true


Write-Host "`n=== Phase 2: Simple Concurrency/Interference Test ==="
Write-Host "Sending 5 requests for Tenant A and Tenant B simultaneously..."

$scriptBlock = {
    param($tid, $model)
    $url = "http://127.0.0.1:8095/v1/completions"
    $baseModel = "qwen2.5-1.5b-instruct"
    $headers = @{ "Content-Type"="application/json"; "X-Tenant-ID"=$tid; "model"=$model }
    $body = @{ model=$baseModel; prompt="Parallel test from $tid"; max_tokens=10 } | ConvertTo-Json
    
    try {
        $sw = [System.Diagnostics.Stopwatch]::StartNew()
        Invoke-RestMethod -Uri $url -Method Post -Headers $headers -Body $body -TimeoutSec 20 | Out-Null
        $sw.Stop()
        return "$tid : Success : $($sw.ElapsedMilliseconds)ms"
    } catch {
        return "$tid : Failed : $($_.Exception.Message)"
    }
}

# Launch jobs
$jobs = @()
1..3 | ForEach-Object { $jobs += Start-Job -ScriptBlock $scriptBlock -ArgumentList "tenant-a", "qwen-code-lora-tenant" }
1..3 | ForEach-Object { $jobs += Start-Job -ScriptBlock $scriptBlock -ArgumentList "tenant-b", "qwen-code-lora-tenant-b" }

Write-Host "Waiting for 6 background jobs..."
$jobs | Receive-Job -Wait | ForEach-Object { Write-Host $_ }
$jobs | Remove-Job

