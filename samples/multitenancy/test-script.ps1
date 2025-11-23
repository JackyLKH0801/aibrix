$url = "http://127.0.0.1:8095/v1/completions"
$model = "qwen2.5-1.5b-instruct"

Write-Host "Sending requests to $url for model $model..."

# Tenant A
Write-Host "Sending request for Tenant A..."
$headersA = @{ "Content-Type" = "application/json"; "X-Tenant-ID" = "tenant-a" }
$bodyA = @{ model = $model; prompt = "Write a hello world function in Python"; max_tokens = 50 } | ConvertTo-Json
try {
    $responseA = Invoke-RestMethod -Uri $url -Method Post -Headers $headersA -Body $bodyA
    Write-Host "Response A:"
    $responseA | ConvertTo-Json -Depth 5
} catch {
    Write-Host "Error Tenant A: $_"
}

# Tenant B
Write-Host "Sending request for Tenant B..."
$headersB = @{ "Content-Type" = "application/json"; "X-Tenant-ID" = "tenant-b" }
$bodyB = @{ model = $model; prompt = "Write a hello world function in Go"; max_tokens = 50 } | ConvertTo-Json
try {
    $responseB = Invoke-RestMethod -Uri $url -Method Post -Headers $headersB -Body $bodyB
    Write-Host "Response B:"
    $responseB | ConvertTo-Json -Depth 5
} catch {
    Write-Host "Error Tenant B: $_"
}

# Default Tenant
Write-Host "Sending request for Default Tenant..."
$headersDefault = @{ "Content-Type" = "application/json" }
$bodyDefault = @{ model = $model; prompt = "Write a hello world function in Java"; max_tokens = 50 } | ConvertTo-Json
try {
    $responseDefault = Invoke-RestMethod -Uri $url -Method Post -Headers $headersDefault -Body $bodyDefault
    Write-Host "Response Default:"
    $responseDefault | ConvertTo-Json -Depth 5
} catch {
    Write-Host "Error Default: $_"
}
