package transport

import clientmultipart "github.com/park285/iris-client-go/v2/internal/client/multipart"

type multipartBodyFactory = clientmultipart.BodyFactory

func newMultipartBodyFactory(metadataBytes []byte, images [][]byte, contentTypes []string) (*multipartBodyFactory, error) {
	return clientmultipart.NewBodyFactory(generateMultipartBoundary(), metadataBytes, images, contentTypes)
}

func validateReplyImages(images [][]byte) error {
	return clientmultipart.ValidateReplyImages(images)
}
