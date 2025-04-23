package contracts

type Converter interface {
	Convert(inputDir string, outputDir string) error
}
