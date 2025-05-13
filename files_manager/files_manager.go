package files_manager

import (
	"os"
	"path/filepath"
	"strings"
	"tiff2pdf/contracts"
)

type TIFFfolder = contracts.TIFFfolder
type ConvertedFolder = contracts.ConvertedFolder
type BoxFolder = contracts.BoxFolder

// func CheckProvidedDirs(inputRootDir string, outputDir string) error {
// 	if inputRootDir == "" || outputDir == "" {
// 		return fmt.Errorf("input and output directories required")
// 	}

// 	if stat, err := os.Stat(inputRootDir); err != nil || !stat.IsDir() {
// 		return fmt.Errorf("input directory does not exist or is not a directory")
// 	}

// 	if stat, err := os.Stat(outputDir); err != nil || !stat.IsDir() {
// 		return fmt.Errorf("output directory does not exist or is not a directory")
// 	}

// 	if inputRootDir == outputDir {
// 		return fmt.Errorf("input and output directories must be different")
// 	}
// 	if strings.Contains(inputRootDir, outputDir) || strings.Contains(outputDir, inputRootDir) {
// 		return fmt.Errorf("input and output directories must not be subdirectories of each other")
// 	}
// 	if !strings.Contains(inputRootDir, "Finalized") {
// 		return fmt.Errorf("input directory name must be 'Finalized', check the path")
// 	}
// 	return nil
// }

// func GetSubDirs(root string) ([]string, error) {
// 	entries, err := os.ReadDir(root)
// 	if err != nil {
// 		return nil, err
// 	}
// 	subDirs := make([]string, 0, len(entries))
// 	for _, entry := range entries {
// 		if entry.IsDir() {
// 			subDirPath := filepath.Join(root, entry.Name())
// 			subDirs = append(subDirs, subDirPath)
// 		}
// 	}
// 	return subDirs, nil
// }

func GetTIFFPaths(dir string) ([]string, int64, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, 0, err
	}
	tiffFiles := make([]string, 0, len(entries))
	var size int64 = 0
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), "._") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == ".tiff" || ext == ".tif" {
			tiffFiles = append(tiffFiles, filepath.Join(dir, entry.Name()))
			info, _ := entry.Info()
			size += info.Size()
		}
	}
	return tiffFiles, size, nil
}

func GetTIFFFolders(rootFolder string) ([]TIFFfolder, error) {

	subDirs, _ := os.ReadDir(rootFolder)

	tiffFoldersCount := len(subDirs)
	if tiffFoldersCount == 0 {
		return []TIFFfolder{}, nil
	}
	tiffFolders := make([]TIFFfolder, 0, len(subDirs))

	for _, entry := range subDirs {
		if !entry.IsDir() {
			continue
		}
		subDirPath := filepath.Join(rootFolder, entry.Name())
		tiffFiles, size, _ := GetTIFFPaths(subDirPath)
		if len(tiffFiles) == 0 {
			continue
		}
		tiffFolders = append(tiffFolders, TIFFfolder{
			TiffFilesPaths: tiffFiles,
			Name:           entry.Name(),
			Path:           subDirPath,
			TiffFilesSize:  size,
		})
	}
	return tiffFolders, nil
}

// func ResolveBoxFolder(boxFolderPath string) (*BoxFolder, error) {

// 	entries, err := os.ReadDir(boxFolderPath)
// 	if err != nil {
// 		return &BoxFolder{}, fmt.Errorf("failed to read directory: %v", err)
// 	}
// 	subFoldersCount := len(entries)
// 	if subFoldersCount == 0 {
// 		return &BoxFolder{}, fmt.Errorf("box folder is empty")
// 	}

// 	box := BoxFolder{
// 		Name: filepath.Base(filepath.Clean(boxFolderPath)),
// 		Path: boxFolderPath,
// 	}

// 	hasFinalized := false
// 	hasConverted := false

// 	for _, entry := range entries {
// 		if !entry.IsDir() {
// 			continue
// 		}
// 		subFolderName := strings.ToLower(entry.Name())

// 		if strings.Contains(subFolderName, string(contracts.Finalized)) {
// 			hasFinalized = true
// 			box.FinalizedFolder = resolveFinalizedFolder(filepath.Join(boxFolderPath, entry.Name()))
// 		}
// 		if strings.Contains(subFolderName, string(contracts.Converted)) {
// 			hasConverted = true
// 			box.ConvertedFolder = ConvertedFolder{
// 				Entries: []string{},
// 				Path:    filepath.Join(boxFolderPath, entry.Name()),
// 			}
// 		}
// 	}
// 	if !hasFinalized && !hasConverted {
// 		return nil, fmt.Errorf("box folder does not contain 'Finalized' or 'Converted' subfolders")
// 	}
// 	return &box, nil
// }

// func resolveFinalizedFolder(folderPath string) []TIFFfolder {

// 	subDirs, _ := os.ReadDir(folderPath)

// 	tiffFoldersCount := len(subDirs)
// 	if tiffFoldersCount == 0 {
// 		return []TIFFfolder{}
// 	}
// 	tiffFolders := make([]TIFFfolder, 0, len(subDirs))

// 	for _, entry := range subDirs {
// 		if !entry.IsDir() {
// 			continue
// 		}
// 		subDirPath := filepath.Join(folderPath, entry.Name())
// 		tiffFiles, size, _ := getTIFFPaths(subDirPath)
// 		if len(tiffFiles) == 0 {
// 			continue
// 		}
// 		tiffFolders = append(tiffFolders, TIFFfolder{
// 			TiffFilesPaths: tiffFiles,
// 			Name:           entry.Name(),
// 			Path:           subDirPath,
// 			TiffFilesSize:  size,
// 		})
// 	}
// 	return tiffFolders
// }
