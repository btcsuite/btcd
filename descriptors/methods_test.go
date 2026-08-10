package descriptors

const (
	testXpub1 = "[e81a5744/48'/0'/0'/2']xpub6Duv8Gj9gZeA3sUo5nUMPEv6FZ81G" +
		"Hn3feyaUej5KqcjPKsYLww4xBX4MmYZUPX5NqzaVJWYdYZwGLECtgQruG4Fk" +
		"ZMh566RkfUT2pbzsEg/<0;1>/*"
	testXpub2 = "[3c157b79/48'/0'/0'/2']xpub6DdSN9RNZi3eDjhZWA8PJ5mSuWgfm" +
		"PdBduXWzSP91Y3GxKWNwkjyc5mF9FcpTFymUh9C4Bar45b6rWv6Y5kSbi9yJ" +
		"DjuJUDzQSWUh3ijzXP/<0;1>/*"

	testTr = "tr(" + testXpub1 + ",and_v(v:pk(" + testXpub2 +
		"),older(65535)))#lg9nqqhr"
)
