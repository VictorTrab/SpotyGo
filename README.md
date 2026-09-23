# SpotyGo

SpotyGo reproduce Spotify en la computadora desde la terminal. Usa una interfaz en Go para los controles y [librespot](https://github.com/librespot-org/librespot) como motor de audio local. Requiere Spotify Premium.

## Instalar en Windows

Necesitas Go 1.25 o posterior y Rust/Cargo para compilar librespot. Desde la carpeta del proyecto, ejecuta:

```powershell
.\scripts\install.ps1 -ClientId "tu-client-id"
```

El instalador coloca `spotygo.exe` en el PATH del usuario e instala librespot en `%LOCALAPPDATA%\SpotyGo\librespot`. La compilación de librespot puede tardar varios minutos. Si la terminal actual no reconoce `spotygo`, abre otra.

Registra `http://127.0.0.1:8989/callback` como Redirect URI en tu aplicación de [Spotify for Developers](https://developer.spotify.com/dashboard). SpotyGo usa OAuth PKCE para los controles. Librespot abre su propia autorización de Spotify la primera vez que inicia y guarda sus credenciales en `%LOCALAPPDATA%\SpotyGo\cache`.

Después ejecuta desde cualquier carpeta:

```powershell
spotygo
```

SpotyGo inicia el motor local y transfiere la reproducción a esta computadora cuando el dispositivo aparece en Spotify Connect. El audio sale por el dispositivo de sonido predeterminado de Windows. Al cerrar SpotyGo, se detiene el motor local.

Para iniciar o comprobar la sesión de los controles:

```powershell
spotygo login
```

Si cambias de aplicación de Spotify, usa `spotygo login --client-id NUEVO_ID`. El Client ID se guarda en la configuración del usuario y los tokens de la interfaz en el almacén de credenciales del sistema operativo. `SPOTIFY_CLIENT_ID` también está disponible.

## Controles

| Tecla | Acción |
| --- | --- |
| `Espacio` | Reproducir o pausar |
| `/` | Buscar canciones; `Enter` busca y reproduce la seleccionada |
| `n` / `p` | Siguiente / anterior |
| `+` / `-` | Subir / bajar volumen en pasos de 5 % |
| `d` | Mostrar dispositivos; `j`/`k` o flechas para elegir, `Enter` para transferir |
| `q` / `Ctrl+C` | Salir |

Los cambios rápidos de volumen se agrupan durante 180 ms. Las órdenes de reproducción se envían una a la vez para que las pulsaciones repetidas no saturen la API. El estado se actualiza cada siete segundos y las respuestas 429 respetan `Retry-After`.

El registro del motor de audio está en `%LOCALAPPDATA%\SpotyGo\librespot.log`. Si ya tienes un binario de librespot, puedes indicar su ruta mediante `SPOTYGO_LIBRESPOT`.

La biblioteca y las playlists descritas en [la especificación](docs/spotify-remote-cli-go.md) siguen pendientes.
