package multipart

func validateReplyImages(images [][]byte) error {
	return ValidateReplyImages(images)
}

func validateReplyMultipartEnvelope(metadataBytes []byte, bodyLength int64) error {
	return ValidateReplyMultipartEnvelope(metadataBytes, bodyLength)
}
