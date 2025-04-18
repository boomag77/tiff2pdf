package files_manager

import (
	"fmt"
	"os"
	"path/filepath"
)

type ImagesDirStat struct {
	TiffFilesPaths []string
	DirName        string
	DirPath        string
	TiffFilesSize  int64
	TotalEntries   int
	TiffFilesCount int
	SubDirsCount   int
}

func CheckProvidedDirs(inputRootDir, outputDir string) error {
	if inputRootDir == "" || outputDir == "" {
		return fmt.Errorf("input and output directories required")
	}

	if stat, err := os.Stat(inputRootDir); err != nil || !stat.IsDir() {
		return fmt.Errorf("input directory does not exist or is not a directory")
	}

	if stat, err := os.Stat(outputDir); err != nil || !stat.IsDir() {
		return fmt.Errorf("output directory does not exist or is not a directory")
	}
	return nil
}

func GetSubDirs(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	subDirs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			subDirPath := filepath.Join(root, entry.Name())
			subDirs = append(subDirs, subDirPath)
		}
	}
	return subDirs, nil
}


