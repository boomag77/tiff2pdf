package files_manager

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type TIFFdir struct {
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

	if inputRootDir == outputDir {
		return fmt.Errorf("input and output directories must be different")
	}
	if strings.Contains(inputRootDir, outputDir) || strings.Contains(outputDir, inputRootDir) {
		return fmt.Errorf("input and output directories must not be subdirectories of each other")
	}
	if !strings.Contains(inputRootDir, "Finalized") {
		return fmt.Errorf("input directory name must be 'Finalized', check the path")
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

func getTIFFPaths(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	tiffFiles := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), "._") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == ".tiff" || ext == ".tif" {
			tiffFiles = append(tiffFiles, filepath.Join(dir, entry.Name()))
		}
	}
	return tiffFiles, nil
}

func createTIFFDirsMap(root string) (map[string]TIFFdir, error) {
	// assume root contains tiff files
	rootDir := TIFFdir{
		TiffFilesPaths: []string{},
		DirName:        filepath.Base(filepath.Clean(root)),
		DirPath:        root,
		TiffFilesSize:  0,
		TotalEntries:   0,
		TiffFilesCount: 0,
		SubDirsCount:   0,
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	rootDir.TotalEntries = len(entries)

	dirs := make(map[string]TIFFdir, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			rootDir.SubDirsCount++
			dirName := entry.Name()
			dirPath := filepath.Join(root, dirName)
			currDir := TIFFdir{
				TiffFilesPaths: []string{},
				DirName:        entry.Name(),
				DirPath:        dirPath,
				TiffFilesSize:  0,
				TotalEntries:   0,
				TiffFilesCount: 0,
				SubDirsCount:   0,
			}
			subEntries, err := os.ReadDir(dirPath)
			if err != nil {
				return nil, err
			}
			currDir.TotalEntries = len(subEntries)
			for _, subEntry := range subEntries {
				if subEntry.IsDir() {
					currDir.SubDirsCount++
					continue
				}
				if strings.HasPrefix(subEntry.Name(), "._") {

					continue
				}
				ext := strings.ToLower(filepath.Ext(subEntry.Name()))
				if ext == ".tiff" || ext == ".tif" {
					currDir.TiffFilesPaths = append(currDir.TiffFilesPaths, filepath.Join(dirPath, subEntry.Name()))

					currDir.TiffFilesCount++
					info, err := subEntry.Info()
					if err != nil {
						return nil, err
					}
					currDir.TiffFilesSize += info.Size()
				}
			}
			if currDir.TiffFilesCount > 0 {
				dirs[dirPath] = currDir
			}
		} else {
			if strings.HasPrefix(entry.Name(), "._") {
				continue
			}
			// Check if the entry is a TIFF file
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			if ext == ".tiff" || ext == ".tif" {
				tiffPath := filepath.Join(root, entry.Name())
				rootDir.TiffFilesPaths = append(rootDir.TiffFilesPaths, tiffPath)
				rootDir.TiffFilesCount++
				info, err := entry.Info()
				if err != nil {
					return nil, err
				}
				rootDir.TiffFilesSize += info.Size()
			}
		}
	}
	if rootDir.TiffFilesCount > 0 {
		dirs[root] = rootDir
	}
	return dirs, nil
}
