package version

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var (
	// Version is the current semantic version of SpotifyGo.
	Current = "v1.0.3"
	// RepoOwner is the GitHub repository owner.
	RepoOwner = "VictorTrab"
	// RepoName is the GitHub repository name.
	RepoName = "SpotyGo"
	// RepoURL is the project website on GitHub.
	RepoURL = "https://github.com/VictorTrab/SpotyGo"
)

// GitHubRelease represents a GitHub release payload.
type GitHubRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
	Body        string `json:"body"`
}

// Info returns a formatted version string with OS and architecture.
func Info() string {
	return fmt.Sprintf("SpotifyGo %s (%s/%s)", Current, runtime.GOOS, runtime.GOARCH)
}

// CheckLatest checks GitHub for the newest available release or tag.
func CheckLatest(ctx context.Context) (*GitHubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", RepoOwner, RepoName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "SpotifyGo/"+Current)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// No formal release yet, check tags
		return checkLatestTag(ctx, client)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("código HTTP inesperado: %d", resp.StatusCode)
	}

	var rel GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

func checkLatestTag(ctx context.Context, client *http.Client) (*GitHubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/tags", RepoOwner, RepoName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "SpotifyGo/"+Current)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("código HTTP al consultar tags: %d", resp.StatusCode)
	}

	var tags []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil, err
	}
	if len(tags) == 0 {
		return &GitHubRelease{TagName: Current}, nil
	}
	return &GitHubRelease{
		TagName: tags[0].Name,
		Name:    tags[0].Name,
		HTMLURL: RepoURL + "/releases/tag/" + tags[0].Name,
	}, nil
}

// RunUpdate performs an in-place update by pulling and rebuilding the latest version.
func RunUpdate() error {
	fmt.Println("=== SpotifyGo Updater ===")
	fmt.Printf("Versión actual: %s\n", Current)
	fmt.Println("Comprobando actualizaciones en GitHub...")

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	latest, err := CheckLatest(ctx)
	targetTag := ""
	if err != nil {
		fmt.Printf("Aviso: no se pudo verificar última release (%v). Procediendo con actualización directa...\n", err)
	} else if latest.TagName != "" {
		targetTag = latest.TagName
		if !IsNewer(latest.TagName, Current) {
			fmt.Printf("Ya estás en la versión más reciente (%s).\n", Current)
			fmt.Print("¿Deseas reinstalar y actualizar a la última versión de desarrollo de main? (s/N): ")
			var answer string
			_, _ = fmt.Scanln(&answer)
			answer = strings.ToLower(strings.TrimSpace(answer))
			if answer != "s" && answer != "si" && answer != "y" && answer != "yes" {
				fmt.Println("Operación cancelada.")
				return nil
			}
		} else {
			fmt.Printf("¡Nueva versión disponible: %s! (Actual: %s)\n", latest.TagName, Current)
		}
	}

	fmt.Println("Descargando la última versión de SpotifyGo...")
	if targetTag != "" {
		if err := DownloadAndApplyUpdate(ctx, targetTag); err == nil {
			fmt.Println("\n[OK] SpotifyGo se actualizó con éxito a la versión " + targetTag + ".")
			fmt.Println("Ejecuta 'spotifygo' para iniciar la aplicación.")
			return nil
		}
	}

	// Fallback to script / go install
	if runtime.GOOS == "windows" {
		exePath, err := os.Executable()
		if err == nil {
			oldPath := exePath + ".old"
			_ = os.Remove(oldPath)
			_ = os.Rename(exePath, oldPath)
		}

		cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command",
			"irm https://raw.githubusercontent.com/VictorTrab/SpotyGo/main/scripts/install.ps1 | iex")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("falló la ejecución del instalador: %w", err)
		}
	} else {
		// Unix / macOS
		cmd := exec.Command("go", "install", "github.com/VictorTrab/SpotyGo@latest")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("falló go install: %w", err)
		}
	}

	// Clean up .old if exists
	if exePath, err := os.Executable(); err == nil {
		_ = os.Remove(exePath + ".old")
		dir := filepath.Dir(exePath)
		_ = os.Remove(filepath.Join(dir, "spotifygo.exe.old"))
	}

	fmt.Println("\n[OK] SpotifyGo se actualizó con éxito.")
	fmt.Println("Ejecuta 'spotifygo' para iniciar la aplicación.")
	return nil
}

// ParseSemver parses a version tag like "v1.2.3" into major, minor, patch ints.
func ParseSemver(v string) (major, minor, patch int, ok bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	parts := strings.Split(v, ".")
	if len(parts) < 1 {
		return 0, 0, 0, false
	}
	var err error
	major, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, 0, false
	}
	if len(parts) > 1 {
		minor, _ = strconv.Atoi(parts[1])
	}
	if len(parts) > 2 {
		p := parts[2]
		if idx := strings.IndexAny(p, "-+"); idx != -1 {
			p = p[:idx]
		}
		patch, _ = strconv.Atoi(p)
	}
	return major, minor, patch, true
}

// IsNewer reports whether remoteTag is strictly newer than localTag.
func IsNewer(remoteTag, localTag string) bool {
	rMaj, rMin, rPat, rOk := ParseSemver(remoteTag)
	lMaj, lMin, lPat, lOk := ParseSemver(localTag)
	if !rOk || !lOk {
		return strings.TrimPrefix(remoteTag, "v") != strings.TrimPrefix(localTag, "v") && remoteTag != ""
	}
	if rMaj != lMaj {
		return rMaj > lMaj
	}
	if rMin != lMin {
		return rMin > lMin
	}
	return rPat > lPat
}

// DownloadAndApplyUpdate fetches the binary asset from GitHub release and replaces the current executable.
func DownloadAndApplyUpdate(ctx context.Context, tag string) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("obtener ruta del ejecutable: %w", err)
	}

	if runtime.GOOS == "windows" {
		zipURL := fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/spotifygo-windows-amd64.zip", RepoOwner, RepoName, tag)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, zipURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", "SpotifyGo/"+Current)

		client := &http.Client{Timeout: 45 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("descargar release: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			zipData, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("leer datos del zip: %w", err)
			}
			zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
			if err != nil {
				return fmt.Errorf("abrir zip: %w", err)
			}

			var exeData []byte
			for _, f := range zr.File {
				if strings.EqualFold(filepath.Base(f.Name), "spotifygo.exe") {
					rc, err := f.Open()
					if err != nil {
						return fmt.Errorf("abrir binario en zip: %w", err)
					}
					exeData, err = io.ReadAll(rc)
					rc.Close()
					if err != nil {
						return fmt.Errorf("extraer binario de zip: %w", err)
					}
					break
				}
			}

			if len(exeData) > 0 {
				oldPath := exePath + ".old"
				_ = os.Remove(oldPath)
				if err := os.Rename(exePath, oldPath); err != nil {
					return fmt.Errorf("renombrar ejecutable actual: %w", err)
				}
				if err := os.WriteFile(exePath, exeData, 0o755); err != nil {
					_ = os.Rename(oldPath, exePath) // rollback
					return fmt.Errorf("escribir nuevo ejecutable: %w", err)
				}
				_ = os.Remove(oldPath)
				return nil
			}
		}
	}

	// Fallback to go install
	cmd := exec.CommandContext(ctx, "go", "install", fmt.Sprintf("github.com/%s/%s@%s", RepoOwner, RepoName, tag))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("falló actualización alternativa: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// Highlight represents a concise summary of a version's changes.
type Highlight struct {
	Version string
	Tagline string
	Points  []string
}

// RecentHighlights returns curated bullet points of recent updates.
func RecentHighlights() []Highlight {
	return []Highlight{
		{
			Version: "v1.0.3",
			Tagline: "Animaciones finas, ecualizador lateral, transiciones fluidas y Zen cinematográfico",
			Points: []string{
				"Barra de reproducción con cabezal pulsante ('●') y onda de brillo continuo ('━')",
				"Espectro ecualizador vertical reubicado a la derecha de la tarjeta Now-Playing",
				"Transición suave entre menús con efecto de barrido en cascada en playlists y canciones",
				"Modo Zen fluido de ~6s con desenfoque continuo multietapa y sin parpadeo de carátula previa",
				"Comprobación automática de actualizaciones al iniciar con modal de novedades (:whatsnew)",
			},
		},
		{
			Version: "v1.0.2",
			Tagline: "Footer contextual, silencio inteligente y temas adaptativos",
			Points: []string{
				"Barra inferior con atajos contextuales según la vista activa",
				"Búsqueda ampliada a 10 resultados con columnas (título, artista, tiempo)",
				"Atajo de silencio ('m') que conmuta y restaura el volumen exacto",
				"Soporte inteligente para temas claros y colores adaptativos",
				"Fondo reactivo 'flow' animado activado por defecto",
				"Acceso directo a temas con 't' y ayuda con '?'",
			},
		},
		{
			Version: "v1.0.1",
			Tagline: "Corrección de autenticación Spotify OAuth",
			Points: []string{
				"Ajuste del callback /login para compatibilidad total con Spotify",
			},
		},
		{
			Version: "v1.0.0",
			Tagline: "Lanzamiento oficial de SpotifyGo",
			Points: []string{
				"Reproductor TUI nativo en Bubble Tea con motor HQ a 320kbps",
				"Carátulas TrueColor en alta definición y modo Zen Sanctuary",
				"Paleta de comandos interactiva con autocompletado en vivo",
			},
		},
	}
}
