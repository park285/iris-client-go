package transport

import clientmultipart "github.com/park285/iris-client-go/internal/client/multipart"

type multipartBodyFactory = clientmultipart.BodyFactory

func newMultipartBodyFactory(metadataBytes []byte, images [][]byte, contentTypes []string) *multipartBodyFactory {
	return clientmultipart.NewBodyFactory(generateMultipartBoundary(), metadataBytes, images, contentTypes)
}

func validateReplyImages(images [][]byte) error {
	return clientmultipart.ValidateReplyImages(images)
}

func validateReplyMultipartEnvelope(metadataBytes []byte, bodyLength int64) error {
	return clientmultipart.ValidateReplyMultipartEnvelope(metadataBytes, bodyLength)
}
