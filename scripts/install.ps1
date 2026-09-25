param(
    [string]$ClientId
)

$ErrorActionPreference = 'Stop'

Write-Host "=== Instalando SpotifyGo ===" -ForegroundColor Green

# 1. Verificar Go
$goCommand = Get-Command go -ErrorAction SilentlyContinue
if ($goCommand) {
    $goExe = $goCommand.Source
} elseif (Test-Path -LiteralPath 'C:\Program Files\Go\bin\go.exe') {
    $goExe = 'C:\Program Files\Go\bin\go.exe'
} else {
    throw 'Go no esta instalado. Instala Go 1.22+ desde https://go.dev/dl/ y vuelve a ejecutar este script.'
}

$installDir = Join-Path ([Environment]::GetFolderPath('UserProfile')) 'go\bin'
New-Item -ItemType Directory -Path $installDir -Force | Out-Null
$target = Join-Path $installDir 'spotifygo.exe'

# 2. Verificar/Instalar librespot
$audioRoot = Join-Path $env:LOCALAPPDATA 'SpotyGo\librespot'
$audioExe = Join-Path $audioRoot 'bin\librespot.exe'

if (-not (Test-Path -LiteralPath $audioExe)) {
    $cargoCommand = Get-Command cargo -ErrorAction SilentlyContinue
    if (-not $cargoCommand) {
        throw 'Falta Rust/Cargo para compilar el motor de audio. Instala Rust desde https://rustup.rs/ y vuelve a ejecutar el instalador.'
    }
    Write-Host 'Instalando librespot para reproducir audio nativo en esta computadora. Puede tardar unos minutos...' -ForegroundColor Cyan
    & $cargoCommand.Source install librespot --version 0.8.0 --locked --root $audioRoot
    if ($LASTEXITCODE -ne 0) {
        throw 'No se pudo instalar librespot.'
    }
}

# 3. Detener instancias activas si las hay para evitar bloqueo de archivo en Windows
Get-Process -Name "*spotifygo*" -ErrorAction SilentlyContinue | Stop-Process -Force

# 4. Obtener codigo fuente y compilar
$projectDir = if ($PSScriptRoot -and (Test-Path (Join-Path $PSScriptRoot '..\main.go'))) {
    (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
} else {
    $null
}

if ($projectDir -and (Test-Path (Join-Path $projectDir 'main.go'))) {
    Write-Host "Compilando SpotifyGo desde $projectDir..." -ForegroundColor Cyan
    Push-Location $projectDir
    try {
        & $goExe build -o $target .
        if ($LASTEXITCODE -ne 0) { throw 'No se pudo compilar SpotifyGo.' }
    } finally {
        Pop-Location
    }
} else {
    Write-Host "Descargando y compilando SpotifyGo desde GitHub (VictorTrab/SpotyGo)..." -ForegroundColor Cyan
    $tempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("spotifygo-build-" + [System.Guid]::NewGuid().ToString("N"))
    try {
        & git clone --depth 1 https://github.com/VictorTrab/SpotyGo.git $tempDir
        Push-Location $tempDir
        & $goExe build -o $target .
        if ($LASTEXITCODE -ne 0) { throw 'No se pudo compilar SpotifyGo.' }
    } finally {
        Pop-Location
        Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# 5. Asegurar PATH
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$entries = @($userPath -split ';' | ForEach-Object { $_.TrimEnd('\') })
if ($entries -notcontains $installDir.TrimEnd('\')) {
    $newPath = if ([string]::IsNullOrWhiteSpace($userPath)) { $installDir } else { $userPath.TrimEnd(';') + ';' + $installDir }
    [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
}
if (@($env:Path -split ';' | ForEach-Object { $_.TrimEnd('\') }) -notcontains $installDir.TrimEnd('\')) {
    $env:Path += ';' + $installDir
}

Write-Host "[OK] SpotifyGo instalado con exito en $target" -ForegroundColor Green
if ($ClientId) {
    & $target login --client-id $ClientId
} else {
    Write-Host "Para iniciar sesion por primera vez, ejecuta: spotifygo login" -ForegroundColor Yellow
}
Write-Host "Para abrir el reproductor, ejecuta: spotifygo" -ForegroundColor Cyan
