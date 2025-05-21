package contracts

type Converter interface {
	Convert(request ConversionRequest) error
}

type ConversionRequest struct {
	Parameters InputFlags
	Folders    []TIFFfolder
}

type ConvertResult struct {
	ImgBuffer []byte
	ImageId   string
	ImgFormat string
	//imgBuffer   io.Reader
	PixelWidth  int
	PixelHeight int
	// drawWidth   float64
	// drawHeight  float64
	//x, y      float64
	PageIndex int
	Gray      bool
	CCITT     bool
}
