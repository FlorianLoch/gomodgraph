package mods

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/rs/zerolog/log"
	"golang.org/x/mod/modfile"
)

type StoreModFileFn func(projectName string, moduleVersion string, modFileContent []byte) error

type ModFileBackend interface {
	ProvideModFilesAndVersions(storeModFile StoreModFileFn) error
}

func Download(backend ModFileBackend, downloadDir string) error {
	return backend.ProvideModFilesAndVersions(func(projectName string, version string, modFileContent []byte) error {
		filePath := path.Join(downloadDir, encodeFilename(projectName, version))

		return os.WriteFile(filePath, modFileContent, 0o600)
	})
}

func encodeFilename(projectName, version string) string {
	// Just generate something unique, we do not need to retrieve the project name from the filename later
	hashedProjectName := sha256.Sum256([]byte(projectName))

	return fmt.Sprintf("%s_%s",
		hex.EncodeToString(hashedProjectName[:]),
		hex.EncodeToString([]byte(version)[:]))
}

func decodeFilename(filename string) (string, error) {
	splits := strings.Split(filename, "_")

	if len(splits) != 2 {
		return "", errors.New("filename does not follow pattern <hex>_<hex>")
	}

	bytez, err := hex.DecodeString(splits[1])
	if err != nil {
		return "", fmt.Errorf("failed decoding version: %w", err)
	}

	return string(bytez), nil
}

type Module struct {
	ModFile *modfile.File
	Version string
}

func ReadModFiles(modFilesDir string) ([]*Module, error) {
	var modFiles []*Module

	dirEntries, err := os.ReadDir(modFilesDir)
	if err != nil {
		return nil, fmt.Errorf("reading contents of download dir: %w", err)
	}

	for _, entry := range dirEntries {
		if !entry.Type().IsRegular() {
			continue
		}

		version, err := decodeFilename(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("decoding filename: %w", err)
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

		modFiles = append(modFiles, &Module{
			ModFile: file,
			Version: version,
		})
	}

	return modFiles, nil
}
