package multipart

import "github.com/park285/iris-client-go/internal/client/randomhex"

type multipartBodyFactory = BodyFactory

func newMultipartBodyFactory(metadataBytes []byte, images [][]byte, contentTypes []string) *BodyFactory {
	return NewBodyFactory(randomhex.Generate("iris-multipart"), metadataBytes, images, contentTypes)
}
