package airflow

import "fmt"

const (
	BaseImageName                = "quay.io/astronomer"
	AstronomerCertifiedImageName = "ap-airflow"
	AstroRuntimeImageName        = "astro-runtime"
)

var (
	FullAstronomerCertifiedImageName = fmt.Sprintf("%s/%s", BaseImageName, AstronomerCertifiedImageName)
	FullAstroRuntimeImageName        = fmt.Sprintf("%s/%s", BaseImageName, AstroRuntimeImageName)
)
