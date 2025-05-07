package contracts

type Converter interface {
	Convert(inputDir string, outputDir string) error
}

type ConversionRequest struct {
	Parameters InputFlags
	Folders    []TIFFfolder
}