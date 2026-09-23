param(
    [string]$ClientId
)

$ErrorActionPreference = 'Stop'
$projectDir = Split-Path -Parent $PSScriptRoot
$goCommand = Get-Command go -ErrorAction SilentlyContinue
if ($goCommand) {
    $goExe = $goCommand.Source
} elseif (Test-Path -LiteralPath 'C:\Program Files\Go\bin\go.exe') {
    $goExe = 'C:\Program Files\Go\bin\go.exe'
} else {
    throw 'Go no está instalado. Instala Go 1.25 o posterior y vuelve a ejecutar este script.'
}

$installDir = Join-Path ([Environment]::GetFolderPath('UserProfile')) 'go\bin'
New-Item -ItemType Directory -Path $installDir -Force | Out-Null
$target = Join-Path $installDir 'spotygo.exe'

Push-Location $projectDir
try {
    & $goExe build -o $target .
    if ($LASTEXITCODE -ne 0) {
        throw 'No se pudo compilar SpotyGo.'
    }
} finally {
    Pop-Location
}

$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$entries = @($userPath -split ';' | ForEach-Object { $_.TrimEnd('\') })
if ($entries -notcontains $installDir.TrimEnd('\')) {
    $newPath = if ([string]::IsNullOrWhiteSpace($userPath)) { $installDir } else { $userPath.TrimEnd(';') + ';' + $installDir }
    [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
}
if (@($env:Path -split ';' | ForEach-Object { $_.TrimEnd('\') }) -notcontains $installDir.TrimEnd('\')) {
    $env:Path += ';' + $installDir
}

Write-Host "SpotyGo instalado en $target"
if ($ClientId) {
    & $target login --client-id $ClientId
    if ($LASTEXITCODE -ne 0) {
        throw 'SpotyGo se instaló, pero no pudo completar el inicio de sesión.'
    }
} else {
    Write-Host 'Si aún no configuraste Spotify, ejecuta: spotygo login --client-id TU_CLIENT_ID'
}
Write-Host 'Después ejecuta: spotygo'
