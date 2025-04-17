package contracts

type Converter interface {
	GetTiffFiles(inputDir string) ([]string, error)
	Convert(inputDir string, outputDir string) error
}
