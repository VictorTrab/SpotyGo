# SpotyGo

SpotyGo es un controlador remoto de Spotify para la terminal. El audio se reproduce en un dispositivo Spotify Connect; la aplicación muestra el estado y envía órdenes mediante la Web API.

## Requisitos

- Go 1.25 o posterior.
- Spotify Premium y una aplicación en [Spotify for Developers](https://developer.spotify.com/dashboard).
- Un dispositivo Spotify Connect abierto (móvil, escritorio o reproductor web).

En la configuración de la aplicación de Spotify registra exactamente `http://127.0.0.1:8989/callback` como Redirect URI. SpotyGo usa OAuth PKCE y solicita los permisos `user-read-playback-state` y `user-modify-playback-state`.

## Ejecutar

En PowerShell:

```powershell
$env:SPOTIFY_CLIENT_ID = "tu-client-id"
go run .
```

La primera ejecución abre el navegador para autorizar el acceso. Los tokens se guardan en el almacén de credenciales del sistema operativo. El Client ID no es un secreto, pero se configura por entorno para que cada usuario use su propia app de Spotify.

## Controles

| Tecla | Acción |
| --- | --- |
| `Espacio` | Reproducir o pausar |
| `n` / `p` | Siguiente / anterior |
| `+` / `-` | Subir / bajar volumen en pasos de 5 % |
| `d` | Mostrar dispositivos; `j`/`k` o flechas para elegir, `Enter` para transferir |
| `q` / `Ctrl+C` | Salir |

Los cambios rápidos de volumen se agrupan durante 180 ms. La aplicación actualiza el estado cada siete segundos y respeta `Retry-After` cuando Spotify limita las solicitudes.

## Alcance

Esta primera versión implementa el control remoto y el selector de dispositivos. La biblioteca, las playlists y la búsqueda local descritas en [la especificación](docs/spotify-remote-cli-go.md) vendrán en una fase posterior.
