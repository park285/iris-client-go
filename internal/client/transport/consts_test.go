package transport

const (
	testExampleBaseURL = "http://127.0.0.1:9"
	testDeliveryQueued = "queued"
	testDeliveryAsync  = "async"
	testRoomA          = "room-a"
	testRoom           = "room"
	testSenderName     = "홍길동"
	testReplyURL       = "http://iris.test/reply"
	testRoomEventsPath = "/rooms/42/events"
	// TestPNGMagic은 SDK가 PNG로 판별하는 최소 magic byte다.
	testPNGMagic = "\x89PNG\r\n\x1a\n"
)
