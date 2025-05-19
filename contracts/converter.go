package contracts

type Converter interface {
	Convert(request ConversionRequest) error
}

type ConversionRequest struct {
	Parameters InputFlags
	Folders    []TIFFfolder
}
