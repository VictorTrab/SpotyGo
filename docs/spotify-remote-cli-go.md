# SpotRemote (TUI) — Controlador Remoto de Spotify en Go
> **Documento de Diseño de Arquitectura y Especificación Técnica**
> *Una alternativa ligera, robusta y ultrarrápida a los reproductores de terminal tradicionales.*

---

## 1. Visión General del Proyecto

### 1.1 El Problema con las alternativas actuales (`spotify_player`, `ncspot`)
1. **Bloqueos HTTP 429 (Too Many Requests):** Al cambiar volumen rápidamente o al tener intervalos de refresco agresivos, las herramientas actuales saturan la Web API de Spotify con decenas de peticiones por segundo, dejando el cliente congelado durante 30+ segundos.
2. **Búsqueda rígida y lenta:** Dependen de consultar la API remota carácter por carácter o carecen de búsqueda difusa (*fuzzy finding*) sobre la biblioteca local guardada.
3. **Sobrecarga de complejidad:** Al intentar actuar simultáneamente como cliente de audio nativo (mediante *librespot*) y como interfaz TUI, los fallos de sincronización de dispositivos y buffers de audio colapsan la experiencia.

### 1.2 La Solución: Controlador Remoto Desacoplado
En lugar de decodificar audio en la terminal, este proyecto se enfoca en ser el **mejor controlador remoto TUI**:
- El audio sigue sonando en la app oficial de Spotify (móvil, app de escritorio de Windows o altavoz inteligente).
- La CLI se encarga exclusivamente de ofrecer:
  - **Cero bloqueos 429:** Throttling y *debouncing* matemático en cada acción de usuario.
  - **Buscador tipo `fzf` instantáneo:** Indexación local en memoria/caché de playlists y canciones guardadas.
  - **Sincronización fluida con el móvil y otros dispositivos:** Detección y cambio instantáneo de dispositivos activos.
  - **Binario nativo en Go:** Inicio en < 50ms, bajo consumo de memoria (< 20MB RAM) y sin dependencias externas.

---

## 2. Stack Tecnológico Recomendado

| Componente | Tecnología | Motivo de Selección |
| :--- | :--- | :--- |
| **Lenguaje** | **Go 1.22+** | Concurrencia sencilla con Goroutines/Channels, compilación cruzada a un único archivo `.exe` para Windows sin dependencias. |
| **Framework TUI** | **Charm Bubble Tea** (`github.com/charmbracelet/bubbletea`) | Patrón arquitectónico Elm (Model-Update-View). Manejo robusto de eventos de teclado, redibujado reactivo y alta fluidez. |
| **Estilos & Layout** | **Charm Lip Gloss** (`github.com/charmbracelet/lipgloss`) | Definición declarativa de colores, bordes, márgenes y temas modernos (soporte 24-bit TrueColor). |
| **Componentes TUI** | **Charm Bubbles** (`github.com/charmbracelet/bubbles`) | Componentes probados para listas, tablas, barras de progreso animadas, textinputs y spinners. |
| **Fuzzy Search** | **Sahilm Fuzzy** (`github.com/sahilm/fuzzy`) | Algoritmo de filtrado difuso idéntico a fzf, ejecutado en memoria a velocidades de microsegundos. |
| **Caché Local** | **SQLite sin CGO** (`modernc.org/sqlite`) o **bbolt** | Almacenamiento local persistente para indexar tracks, playlists y biblioteca del usuario sin requerir compiladores C en Windows. |
| **Auth & API** | **OAuth 2.0 PKCE + HTTP Client nativo** | Servidor local temporal en `127.0.0.1:8989` para captura de tokens; cliente HTTP personalizado con retry inteligente. |

---

## 3. Arquitectura del Sistema

```
┌─────────────────────────────────────────────────────────────┐
│                       Interfaz TUI                          │
│        (Bubble Tea Model-Update-View + Lip Gloss)           │
├──────────────────────────────┬──────────────────────────────┤
│    Barra de Reproducción     │    Buscador Difuso (FZF)     │
│   (Track, Progreso, Vol.)    │    (Filtro en tiempo real)   │
├──────────────────────────────┴──────────────────────────────┤
│                         Core Engine                         │
├──────────────────────────────┬──────────────────────────────┤
│     Debouncer / Throttler    │     Device Manager           │
│   (Retención de 150-200ms)   │   (Detección Spotify Connect)│
├──────────────────────────────┼──────────────────────────────┤
│      Caché Local SQLite      │    Token Bucket Rate Limiter │
│   (Indexación de biblioteca) │   (Control de cuota de API)  │
└──────────────────────────────┴──────────────────────────────┘
                               │
               HTTPS (Spotify Web API v1)
                               │
                               ▼
        ┌──────────────────────────────────────────────┐
        │  Spotify Cloud (Control de Dispositivos)     │
        │  - Teléfono móvil (iOS / Android)            │
        │  - Spotify Desktop / Web Player              │
        └──────────────────────────────────────────────┘
```

---

## 4. Mecanismos Clave para Resolver los Problemas Actuales

### 4.1 Throttling y Debounce Inteligente (Adiós HTTP 429)
- **Problema de origen:** Cuando el usuario sube el volumen o se desplaza por una lista, se emiten decenas de llamadas por segundo que activan el bloqueo de Spotify.
- **Implementación en Go:**
  - Se utiliza un `time.Timer` con canal de reajuste.
  - Al pulsar `+` o `-`, el volumen en la pantalla cambia **inmediatamente** a nivel visual (latencia 0).
  - La petición real `PUT /v1/me/player/volume` se retrasa `180 ms`. Si el usuario pulsa otra tecla durante ese intervalo, el timer se reinicia.
  - Al detenerse, se envía **una sola petición** con el valor acumulado final.
- **Control de cabeceras `Retry-After`:** Si Spotify devuelve un 429, el cliente intercepta la cabecera, pausa todas las solicitudes en cola por los segundos indicados y muestra un aviso visual discreto sin colgar la interfaz.

### 4.2 Buscador Híbrido Ultrarrápido (Local First)
1. **Búsqueda Instantánea en Biblioteca (Offline/Memoria):**
   - Al iniciar la app, se leen en segundo plano las playlists y canciones guardadas del usuario y se almacenan en caché local.
   - Al pulsar `/`, la búsqueda filtra sobre la lista local en memoria: respuestas en **menos de 5 milisegundos**.
2. **Búsqueda Global en Catálogo de Spotify:**
   - Si el usuario pulsa `Enter` o un atajo de búsqueda global, se consulta la API de Spotify con un debounce de `300 ms` tras la última tecla pulsada para evitar peticiones redundantes.

### 4.3 Sincronización Fluida con el Teléfono (Spotify Connect)
- **Autoselección de dispositivo:** Al arrancar, consulta `GET /v1/me/player/devices`. Si el teléfono está activo reproduciendo, se ancla automáticamente como target.
- **Selector emergente:** Presionando `d`, se despliega un modal con todos los dispositivos disponibles con su tipo (Smartphone, Computadora, Altavoz) y estado de conexión para cambiar con las flechas y `Enter`.
- **Refresco bajo demanda:** Solo actualiza el estado cuando ocurre un evento de usuario (pausar, cambiar canción, transferir dispositivo), o con un sondeo suave cada `5 a 8 segundos` (no cada 1 segundo), evitando agotar las cuotas de red.

---

## 5. Diseño de Interfaz de Usuario (Mockup TUI)

```
┌────────────────────────────────────────────────────────────────────────┐
│  ▶  Fire & Desire (E) • Drake                          [ iPhone 15 ]   │
│     Views • rap                                          Vol: 75%      │
│  [==========================----------------------] 1:45 / 3:58  🔀 🔁 │
├────────────────────────────────────────────────────────────────────────┤
│  Playlists (4)      │  Canciones                                       │
│  > Favoritas        │  1.  Sticky - Drake                              │
│    Hip Hop Hits     │  2. ▶ Fire & Desire - Drake                      │
│    Coding Lofi      │  3.  Passionfruit - Drake                        │
│    Gym Phonk        │  4.  One Dance - Drake                           │
├─────────────────────┴──────────────────────────────────────────────────┤
│  🔍 Buscar (fzf): purp_                                                │
│  [Enter] Reproducir  [Space] Pausa  [n/p] Sig/Ant  [d] Disp.  [/] Buscar │
└────────────────────────────────────────────────────────────────────────┘
```

---

## 6. Mapa de Atajos de Teclado (Keymap Ergonómico)

| Tecla | Acción | Descripción |
| :--- | :--- | :--- |
| **`Space`** | Play / Pause | Alterna entre reproducir y pausar sin retardo. |
| **`n` / `p`** | Siguiente / Anterior | Salta a la siguiente o anterior canción. |
| **`+` / `-`** (o `]` / `[`) | Volumen + / - | Ajuste de volumen con debouncing automático. |
| **`j` / `k`** o `↑` / `↓` | Navegar | Desplazarse por canciones, playlists o listas. |
| **`h` / `l`** o `←` / `→` | Cambiar panel | Alterna entre paneles (Playlists, Canciones, Cola). |
| **`Enter`** | Seleccionar | Reproduce el elemento resaltado. |
| **`/`** | Buscar | Abre el buscador difuso con filtro en tiempo real. |
| **`d`** | Selector de dispositivos | Abre la ventana emergente para elegir teléfono, PC, etc. |
| **`s`** | Shuffle | Alterna modo aleatorio. |
| **`r`** | Repeat | Alterna modo repetición (apagado, lista, canción). |
| **`q`** / `Esc` | Salir / Cerrar modal | Cierra ventanas emergentes o sale de la app. |

---

## 7. Fases de Implementación Sugeridas

### Fase 1: Núcleo y Conectividad (1 - 2 días)
- Configurar autenticación OAuth PKCE con `client_id` probado.
- Implementar cliente HTTP con interceptor de rate-limiting (Token Bucket) y debounce de volumen.
- Validar comandos básicos: `Play`, `Pause`, `Next`, `Previous`, `Volume`, `GetDevices`.

### Fase 2: Interfaz TUI Base (2 - 3 días)
- Inicializar proyecto con **Bubble Tea** y **Lip Gloss**.
- Diseñar la barra superior de reproducción con estado en tiempo real.
- Renderizar lista de playlists y canciones activas con soporte para teclado (`j`/`k`, `Enter`).

### Fase 3: Buscador Difuso y Caché Local (2 días)
- Integrar `modernc.org/sqlite` para sincronizar biblioteca en segundo plano.
- Construir el componente de búsqueda con `sahilm/fuzzy`.
- Permitir seleccionar cualquier resultado y enviarlo a la cola de reproducción inmediatamente.

### Fase 4: Selector de Dispositivos y Pulido (1 - 2 días)
- Implementar modal de dispositivos (`d`) con selección interactiva.
- Pruebas intensivas de scroll y teclas rápidas para verificar inmunidad a errores 429.
- Generar ejecutable compilado (`spotremote.exe`) con Go build.

---

## 8. Conclusión

Este enfoque resuelve exactamente las debilidades de las alternativas existentes: delega el trabajo pesado de audio a Spotify y se enfoca en proporcionar una experiencia de consola ágil, estética, que nunca se cuelgue y con una búsqueda verdaderamente cómoda.
