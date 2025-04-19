package contracts

type BoxSubFolderName string

const (
	Finalized BoxSubFolderName = "finalized"
	Converted BoxSubFolderName = "converted"
)

type TIFFfolder struct {
	TiffFilesPaths []string
	Name           string
	Path           string
	TiffFilesSize  int64
}

type ConvertedFolder struct {
	Entries []string
	Path    string
}

type BoxFolder struct {
	FinalizedFolder []TIFFfolder
	PDFPaths        []string
	Failed          []string
	ConvertedFolder ConvertedFolder
	OutputFolder    string
	Name            string
	Path            string
}
