package mods

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path"

	"github.com/rs/zerolog/log"
	"golang.org/x/mod/modfile"
)

type StoreModFileFn func(projectName string, modFileContent []byte) error

type ModFileBackend interface {
	DownloadModFiles(storeModFile StoreModFileFn) error
}

func Download(backend ModFileBackend, downloadDir string) error {
	return backend.DownloadModFiles(func(projectName string, modFileContent []byte) error {
		hashedProjectName := sha256.Sum256([]byte(projectName))
		filePath := path.Join(downloadDir, hex.EncodeToString(hashedProjectName[:]))

		return os.WriteFile(filePath, modFileContent, 0o600)
	})
}

func ReadModFiles(modFilesDir string) ([]*modfile.File, error) {
	var modFiles []*modfile.File

	dirEntries, err := os.ReadDir(modFilesDir)
	if err != nil {
		return nil, fmt.Errorf("reading contents of download dir: %w", err)
	}

	for _, entry := range dirEntries {
		if !entry.Type().IsRegular() {
			continue
		}

		filePath := path.Join(modFilesDir, entry.Name())

		bytez, err := os.ReadFile(filePath)
		if err != nil {
			log.Error().Msgf("Could not read mod file %q: %v", filePath, err)

			continue
		}

		file, err := modfile.Parse(entry.Name(), bytez, nil)
		if err != nil {
			log.Error().Msgf("Could not parse mod file %q: %v", filePath, err)

			continue
		}

		modFiles = append(modFiles, file)
	}

	return modFiles, nil
}
