package version

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var (
	// Version is the current semantic version of SpotifyGo.
	Current = "v1.0.2"
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
	if err != nil {
		fmt.Printf("Aviso: no se pudo verificar última release (%v). Procediendo con actualización directa desde main...\n", err)
	} else if latest.TagName != "" {
		if strings.TrimPrefix(latest.TagName, "v") == strings.TrimPrefix(Current, "v") {
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

	fmt.Println("Descargando y compilando la última versión...")

	// On Windows, prepare by renaming current running binary so file lock is avoided
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
