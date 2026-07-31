package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type Asset struct {
	File      string   `json:"file"`
	CSS       []string `json:"css,omitempty"`
	Integrity string   `json:"integrity,omitempty"`
}

type Manifest map[string]Asset // key = entry source (e.g. "src/index.ts")

var (
	manifestOnce sync.Once
	manifest     Manifest
	manifestErr  error
)

// LoadManifest reads .vite/manifest.json from the static folder.
// Call once at startup (e.g. in main() or boot).
func LoadManifest(staticDir string) error {
	manifestOnce.Do(func() {
		path := filepath.Join(staticDir, "dist", ".vite", "manifest.json")
		data, err := os.ReadFile(path)
		if err != nil {
			manifestErr = err
			return
		}
		var m Manifest
		if err := json.Unmarshal(data, &m); err != nil {
			manifestErr = err
			return
		}
		manifest = m
	})
	return manifestErr
}

// GetJS returns file name + integrity for the main entry.
// In your case: "src/index.ts"
func GetJS(scriptName string) (file, integrity string) {
	if asset, ok := manifest[fmt.Sprintf("src/%s.ts", scriptName)]; ok {
		return asset.File, asset.Integrity
	}
	return "index.js", "" // fallback (dev)
}

// GetCSS returns the first CSS file (you only have one)
func GetCSS(scriptName string) (file, integrity string) {
	if asset, ok := manifest[fmt.Sprintf("src/%s.ts", scriptName)]; ok && len(asset.CSS) > 0 {
		// CSS files are also in manifest if they are imported
		// but in your case they are listed under the entry.
		// We'll resolve them below via GetAssetIntegrity.
		return asset.CSS[0], GetAssetIntegrity(asset.CSS[0])
	}
	return "index.css", ""
}

// GetAssetIntegrity looks up any asset by its *output* filename.
// Vite adds every emitted file as a key in the manifest.
func GetAssetIntegrity(filename string) string {
	if asset, ok := manifest[filename]; ok {
		return asset.Integrity
	}
	// Try with leading "assets/" if you use assetFileNames
	for _, a := range manifest {
		if a.File == filename {
			return a.Integrity
		}
	}
	return ""
}
