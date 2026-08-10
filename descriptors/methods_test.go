package descriptors

const (
	testXpub1 = "[e81a5744/48'/0'/0'/2']xpub6Duv8Gj9gZeA3sUo5nUMPEv6" +
		"FZ81GHn3feyaUej5KqcjPKsYLww4xBX4MmYZUPX5NqzaVJWYdYZwGLECtg" +
		"QruG4FkZMh566RkfUT2pbzsEg/<0;1>/*"
	testXpub2 = "[3c157b79/48'/0'/0'/2']xpub6DdSN9RNZi3eDjhZWA8PJ5mS" +
		"uWgfmPdBduXWzSP91Y3GxKWNwkjyc5mF9FcpTFymUh9C4Bar45b6rWv6Y5" +
		"kSbi9yJDjuJUDzQSWUh3ijzXP/<0;1>/*"

	testTr = "tr(" + testXpub1 + ",and_v(v:pk(" + testXpub2 +
		"),older(65535)))#lg9nqqhr"
)
