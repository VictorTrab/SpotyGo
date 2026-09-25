param(
    [string]$ClientId
)

$ErrorActionPreference = 'Stop'

Write-Host "=== Instalador Automatico de SpotifyGo ===" -ForegroundColor Green

function Refresh-EnvPath {
    $machinePath = [Environment]::GetEnvironmentVariable('Path', 'Machine')
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    $env:Path = "$userPath;$machinePath"
}

# 1. Verificar o Instalar Go
$goExe = $null
$goCommand = Get-Command go -ErrorAction SilentlyContinue
if ($goCommand) {
    $goExe = $goCommand.Source
} elseif (Test-Path 'C:\Program Files\Go\bin\go.exe') {
    $goExe = 'C:\Program Files\Go\bin\go.exe'
} elseif (Test-Path "$env:LOCALAPPDATA\Programs\Go\bin\go.exe") {
    $goExe = "$env:LOCALAPPDATA\Programs\Go\bin\go.exe"
}

if (-not $goExe) {
    Write-Host "Go no esta instalado. Descargando e instalando Go automaticamente..." -ForegroundColor Cyan
    $winget = Get-Command winget -ErrorAction SilentlyContinue
    $installed = $false
    if ($winget) {
        Write-Host "Instalando Go mediante winget..." -ForegroundColor Gray
        try {
            & winget install --id GoLang.Go -e --silent --accept-package-agreements --accept-source-agreements
            Refresh-EnvPath
            $installed = $true
        } catch {}
    }
    
    if (-not $installed -or -not (Get-Command go -ErrorAction SilentlyContinue)) {
        Write-Host "Descargando instalador oficial de Go..." -ForegroundColor Gray
        $goMsi = Join-Path ([System.IO.Path]::GetTempPath()) "go_installer.msi"
        Invoke-WebRequest -Uri "https://go.dev/dl/go1.23.2.windows-amd64.msi" -OutFile $goMsi
        Write-Host "Instalando Go en segundo plano..." -ForegroundColor Gray
        Start-Process msiexec.exe -ArgumentList "/i `"$goMsi`" /qn" -Wait
        Remove-Item $goMsi -Force -ErrorAction SilentlyContinue
        Refresh-EnvPath
    }

    if (Test-Path 'C:\Program Files\Go\bin\go.exe') {
        $goExe = 'C:\Program Files\Go\bin\go.exe'
    } else {
        $goCommand = Get-Command go -ErrorAction SilentlyContinue
        if ($goCommand) { $goExe = $goCommand.Source }
    }

    if (-not $goExe) {
        throw "No se pudo instalar Go automaticamente. Por favor descargalo desde https://go.dev/dl/ e intentalo de nuevo."
    }
    Write-Host "[OK] Go instalado correctamente." -ForegroundColor Green
}

$installDir = Join-Path ([Environment]::GetFolderPath('UserProfile')) 'go\bin'
New-Item -ItemType Directory -Path $installDir -Force | Out-Null
$target = Join-Path $installDir 'spotifygo.exe'

# 2. Verificar o Instalar librespot y Rust
$audioRoot = Join-Path $env:LOCALAPPDATA 'SpotyGo\librespot'
$audioExe = Join-Path $audioRoot 'bin\librespot.exe'

if (-not (Test-Path -LiteralPath $audioExe)) {
    $cargoCommand = Get-Command cargo -ErrorAction SilentlyContinue
    $cargoExe = if ($cargoCommand) { $cargoCommand.Source } else { $null }
    if (-not $cargoExe -and (Test-Path "$env:USERPROFILE\.cargo\bin\cargo.exe")) {
        $cargoExe = "$env:USERPROFILE\.cargo\bin\cargo.exe"
    }

    if (-not $cargoExe) {
        Write-Host "Rust/Cargo no esta instalado. Instalando Rust automaticamente..." -ForegroundColor Cyan
        $rustupExe = Join-Path ([System.IO.Path]::GetTempPath()) "rustup-init.exe"
        Invoke-WebRequest -Uri "https://win.rustup.rs/x86_64" -OutFile $rustupExe
        Write-Host "Configurando toolchain de Rust (esto puede tardar unos momentos)..." -ForegroundColor Gray
        Start-Process -FilePath $rustupExe -ArgumentList "-y --default-toolchain stable" -Wait -NoNewWindow
        Remove-Item $rustupExe -Force -ErrorAction SilentlyContinue
        $cargoExe = "$env:USERPROFILE\.cargo\bin\cargo.exe"
        Refresh-EnvPath
    }

    if (-not (Test-Path $cargoExe)) {
        throw "No se pudo instalar Rust/Cargo automaticamente. Instala Rust desde https://rustup.rs/ e intentalo de nuevo."
    }

    Write-Host "Compilando librespot 0.8.0 para streaming de audio local. Puede tardar unos minutos..." -ForegroundColor Cyan
    & $cargoExe install librespot --version 0.8.0 --locked --root $audioRoot
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path $audioExe)) {
        throw "No se pudo compilar librespot."
    }
    Write-Host "[OK] Motor de audio librespot listo." -ForegroundColor Green
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
    Write-Host "Compilando SpotifyGo desde codigo local..." -ForegroundColor Cyan
    Push-Location $projectDir
    try {
        & $goExe build -o $target .
        if ($LASTEXITCODE -ne 0) { throw 'No se pudo compilar SpotifyGo.' }
    } finally {
        Pop-Location
    }
} else {
    Write-Host "Descargando ultima version de SpotifyGo desde GitHub..." -ForegroundColor Cyan
    $tempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("spotifygo-build-" + [System.Guid]::NewGuid().ToString("N"))
    $zipPath = "$tempDir.zip"
    try {
        Invoke-WebRequest -Uri "https://github.com/VictorTrab/SpotyGo/archive/refs/heads/main.zip" -OutFile $zipPath
        Expand-Archive -Path $zipPath -DestinationPath $tempDir -Force
        $sourceDir = Join-Path $tempDir "SpotyGo-main"
        Push-Location $sourceDir
        & $goExe build -o $target .
        if ($LASTEXITCODE -ne 0) { throw 'No se pudo compilar SpotifyGo.' }
    } finally {
        Pop-Location
        Remove-Item -Path $zipPath -Force -ErrorAction SilentlyContinue
        Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# 5. Asegurar PATH en la sesion y usuario
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
