package signing

const PathReply = "/reply"
const emptyBodySHA256Hex = EmptyBodySHA256Hex

type hmacSigner = HMACSigner

func newHMACSigner(secret string) *hmacSigner {
	return NewHMACSigner(secret)
}

func signIrisCanonicalWithSigner(signer *hmacSigner, method, path, timestamp, nonce, bodySHA256 string) (string, error) {
	return SignIrisCanonicalWithSigner(signer, method, path, timestamp, nonce, bodySHA256)
}

func signIrisRequestWithBodySHA256(secret, method, path, timestamp, nonce, bodySHA256 string) (string, error) {
	return SignIrisCanonicalWithSigner(NewHMACSigner(secret), method, path, timestamp, nonce, bodySHA256)
}
