package envfile

import (
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// Load applies the nearest .env file. Search starts at the working
// directory and walks toward the repository root. A variable already
// set in the environment is left unchanged. No file is not an error.
func Load() error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	for {
		loaded, err := loadIfPresent(filepath.Join(dir, ".env"))
		if err != nil || loaded {
			return err
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			_, err = loadIfPresent(filepath.Join(dir, "backend", ".env"))
			return err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}
		dir = parent
	}
}

func loadIfPresent(path string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, godotenv.Load(path)
}
