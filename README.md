# SpotifyGo

<p align="center">
  <img src="assets/intro.gif" alt="SpotifyGo Intro Animation" width="700" />
</p>

<p align="center">
  <strong>El reproductor y cliente de Spotify en terminal (TUI) de nueva generación, ultra-ligero y de alta fidelidad.</strong>
</p>

<p align="center">
  <a href="https://golang.org"><img src="https://img.shields.io/badge/Go-1.22+-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go Version" /></a>
  <a href="https://charm.sh"><img src="https://img.shields.io/badge/Charm-Bubble%20Tea%20v2-7D56F4?style=for-the-badge" alt="Bubble Tea" /></a>
  <a href="https://charm.sh"><img src="https://img.shields.io/badge/Charm-Lip%20Gloss-EE5396?style=for-the-badge" alt="Lip Gloss" /></a>
  <a href="https://github.com/librespot-org/librespot"><img src="https://img.shields.io/badge/Audio-librespot%200.8-DEA584?style=for-the-badge&logo=rust&logoColor=white" alt="librespot" /></a>
  <a href="https://developer.spotify.com"><img src="https://img.shields.io/badge/Spotify-Web%20API-1ED760?style=for-the-badge&logo=spotify&logoColor=white" alt="Spotify API" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue.svg?style=for-the-badge" alt="License: MIT" /></a>
</p>

---

## ⚡ Instalación Rápida (1 Solo Comando)

En tu terminal de **PowerShell** (ejecutada como usuario normal), copia y pega este comando para descargar, compilar e instalar **SpotifyGo** automáticamente:

```powershell
irm https://raw.githubusercontent.com/VictorTrab/SpotyGo/main/scripts/install.ps1 | iex
```

> **Requisitos Previos:**
> - [Go 1.22+](https://go.dev/dl/) instalado en el sistema.
> - [Rust / Cargo](https://rustup.rs/) (utilizado automáticamente por el instalador para compilar el motor de audio `librespot` si aún no lo tienes).
> - Cuenta de **Spotify Premium** (requerida por la API de Spotify Connect para streaming de audio nativo).

---

## 📸 Capturas de Pantalla

### Vista Principal (Reproductor TUI + Tarjeta Now-Playing + Playlists)
Fondo degradado ambiental reactivo a la carátula, información completa de álbum y año, barra de progreso y navegación por tus listas.

<p align="center">
  <img src="assets/preview-main.png" alt="SpotifyGo Main View" width="850" />
</p>

### Modo Zen / Big Cover Art (`/art` o tecla `z`)
Sanctuary inmersivo con carátula TrueColor en alta resolución y cielo nocturno con estrellas titilantes en tiempo real.

<p align="center">
  <img src="assets/preview-zen.png" alt="SpotifyGo Zen Mode" width="850" />
</p>

---

## ✨ Características Principales

- 🎵 **Reproducción Nativa de Alta Fidelidad (HQ 320k):** Motor de streaming integrado con soporte para 96k, 160k y 320k seleccionable en caliente mediante `/quality`.
- 🎨 **Carátulas TrueColor en ANSI (`▀`):** Rasterizador de semibloques que despliega carátulas nítidas directamente en la terminal.
- 🌌 **Modo Zen Sanctuary (`z` o `/art`):** Experiencia minimalista con carátula ampliada y un cielo de estrellas vivas generadas proceduralmente.
- 🌈 **Fondo con Degradado Reactivo:** Iluminación dinámica que extrae los colores dominantes del álbum y baña la terminal suavemente (modos `gradient`, `flow`, `dark` y `transparent` mediante `/bg`).
- ⚡ **Caché en 2 Niveles (0.0 ms):** Sistema de caché en memoria RAM y disco SSD para metadatos y carátulas. Cero llamadas redundantes y adiós definitivo a los bloqueos HTTP 429.
- 🍃 **Eco-Power Zero-Idle:** Detección de foco de terminal (`xterm 1004h`). Cuando la ventana no está activa, el consumo de CPU desciende al **0.0%** sin detener la música.
- ⌨️ **Navegación Fluida y Paleta de Comandos (`/`):** Control estilo Vim (`j`/`k`, `p`, `Space`), búsqueda difusa en tiempo real y buscador de comandos interactivo.
- 🔒 **Seguridad y Privacidad Absoluta:** Autenticación mediante **OAuth 2.0 PKCE** local. Ninguna credencial, contraseña ni token personal sale de tu equipo.

---

## 🚀 Primeros Pasos

### 1. Iniciar Sesión por Primera Vez
Ejecuta en tu terminal:

```bash
spotifygo login
```

Se abrirá una ventana de navegador solicitando autorización en Spotify. La aplicación usa el flujo estándar y seguro de OAuth 2.0 PKCE en bucle local (`http://127.0.0.1:8989/callback`).

### 2. Abrir SpotifyGo
Una vez autenticado, simplemente ejecuta:

```bash
spotifygo
```

SpotifyGo iniciará el motor de audio en segundo plano y comenzará la reproducción en tu computadora de forma inmediata.

---

## 🎮 Controles y Atajos de Teclado

### Controles Básicos
| Tecla | Acción |
| :--- | :--- |
| `Espacio` | Reproducir / Pausar |
| `n` / `→` | Siguiente canción |
| `p` / `←` | Canción anterior |
| `+` / `-` | Subir / Bajar volumen (en pasos de 5%) |
| `j` / `↓` | Mover cursor hacia abajo |
| `k` / `↑` | Mover cursor hacia arriba |
| `Enter` | Abrir playlist seleccionada / Reproducir canción |
| `Esc` | Volver a la vista de playlists / Cerrar modal |
| `z` | Alternar Modo Zen (`/art`) con cielo estrellado |
| `S` | Búsqueda directa de canciones |
| `/` | Abrir Paleta de Comandos interactiva |
| `?` | Ayuda y atajos |
| `q` / `Ctrl+C` | Salir de SpotifyGo |

### Comandos Rápidos (`/`)
Escribe `/` en cualquier momento para desplegar la paleta de comandos:

- `/art` — Alterna la vista Zen con arte a gran escala y cielo estrellado.
- `/bg` — Alterna modos de fondo (`gradient`, `flow`, `dark`, `transparent`).
- `/theme` — Selector de temas de color (`Spotify`, `Synthwave`, `Tokyo Night`, `Nord`).
- `/quality` — Selector de calidad de streaming de audio (`320k`, `160k`, `96k`).
- `/devices` — Lista y selector de dispositivos Spotify Connect disponibles.
- `/search <texto>` — Búsqueda instantánea en catálogo de Spotify.
- `/clear` — Limpia la caché local de carátulas y canciones.

---

## 🛠️ Compilación Manual desde el Código Fuente

Si prefieres clonar y compilar el proyecto manualmente:

```powershell
# Clonar el repositorio
git clone https://github.com/VictorTrab/SpotyGo.git
cd SpotyGo

# Compilar todas las pruebas unitarias (64 tests)
go test -v ./...

# Compilar el binario ejecutable
go build -o spotifygo.exe .

# Mover a tu carpeta de binarios en el PATH (opcional)
Copy-Item spotifygo.exe "$env:USERPROFILE\go\bin\spotifygo.exe"
```

Para más detalles sobre los componentes internos y el flujo de datos, consulta la [Documentación de Arquitectura](docs/architecture.md).

---

## 📄 Licencia

Este proyecto está bajo la licencia libre y abierta **MIT**. Consulta el archivo [LICENSE](LICENSE) para más detalles.

---

<p align="center">
  Hecho con ❤️ para la comunidad de terminal por <a href="https://github.com/VictorTrab">VictorTrab</a>
</p>
