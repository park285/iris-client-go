package multipart

import "github.com/park285/iris-client-go/v2/internal/client/randomhex"

type multipartBodyFactory = BodyFactory

func newMultipartBodyFactory(metadataBytes []byte, images [][]byte, contentTypes []string) (*BodyFactory, error) {
	return NewBodyFactory(randomhex.Generate(), metadataBytes, images, contentTypes)
}
