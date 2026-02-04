$url = "http://127.0.0.1:8095/v1/completions"
$baseModel = "qwen2.5-1.5b-instruct"

# Helper to send request with specific tenant and model
function Test-Tenant {
    param($name, $tid, $routeModelName, $expectSuccess=$true)
    Write-Host "`n[TEST] Tenant: $name ($tid)"
    Write-Host "       Routing via Header: model=$routeModelName"
    Write-Host "       Request Body Model: model=$baseModel"
    
    $headers = @{ 
        "Content-Type" = "application/json"; 
        "X-Tenant-ID" = $tid;
        # Envoy routes key off this header:
        "model" = $routeModelName
    }

    # vLLM/Runtime needs to know the ACTUAL running model name
    $body = @{ model = $baseModel; prompt = "Hello from $name"; max_tokens = 50 } | ConvertTo-Json
    
    try {
        $resp = Invoke-RestMethod -Uri $url -Method Post -Headers $headers -Body $body
        if ($expectSuccess) { 
            Write-Host "Response: Success" 
            # Write-Host ($resp | ConvertTo-Json -Depth 2)
        }
        else { Write-Host "WARNING: Unexpected Success!" }
    } catch {
        # Check for 404 (Not Found) or 503 (Service Unavailable) which indicates routing blocked/missing
        if (-not $expectSuccess) {
             Write-Host "SUCCESS: Access Denied as expected. (Error: $($_.Exception.Message))"
        }
        else {
             Write-Host "Error: $($_.Exception.Message)"
             if ($_.ErrorDetails) { Write-Host "Details: $($_.ErrorDetails.Message)" }
        }
    }
}

# 1. Tenant A (Should Succeed)
Test-Tenant -name "Tenant A" -tid "tenant-a" -routeModelName "qwen-code-lora-tenant" -expectSuccess $true

# 2. Tenant B (Should Succeed)
Test-Tenant -name "Tenant B" -tid "tenant-b" -routeModelName "qwen-code-lora-tenant-b" -expectSuccess $true

# 3. Hacker (Should Fail - Accessing Tenant A`'s model with wrong TID)
Test-Tenant -name "Hacker" -tid "hacker-tenant" -routeModelName "qwen-code-lora-tenant" -expectSuccess $false

