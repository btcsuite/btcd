package electrum

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
)

//func TestBlockchainAddressGetBalance(t *testing.T) {
//	node := NewNode()
//	err := node.ConnectTCP("electrum.bitaroo.net:50002")
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	header, err := node.BlockchainGetBlockHeader(700000)
//	if err != nil {
//		t.Fatal(err)
//	}
//	fmt.Println("header", header)
//}

func TestScriptStatus(t *testing.T) {
	//address := "bc1qhl5ez6a3srxs2djdpeneasrv363udyyn4vggdt"
	//addr, err := btcutil.DecodeAddress(address, &chaincfg.MainNetParams)
	//if err != nil {
	//	t.Fatal(err)
	//}
	//pkScript, err := txscript.PayToAddrScript(addr)
	//if err != nil {
	//	t.Fatal(err)
	//}
	//hash := sha256.Sum256(pkScript)
	//// Reverse the hash.
	//sort.SliceStable(hash[:], func(i, j int) bool {
	//	return i > j
	//})

	//str := fmt.Sprintf("%s:%d:", "02f50051ae3cf31ffb83f7b09e72a285d1504222ee211d473f2098fa76e8befa", 766888)
	//bytes, err := hex.DecodeString(str)
	//if err != nil {
	//	t.Fatal(err)
	//}

	str := fmt.Sprintf("%s:%d:", "97c702b9fc4fe07630e7197bc6682ca34e83099e7a4062c927129e6ffea75a47", 129869)

	hash := sha256.Sum256([]byte(str))
	fmt.Println(hex.EncodeToString(hash[:]))
	fmt.Printf("%x\n", hash[:])
}

func TestBlockchainScriptHashGetHistory(t *testing.T) {
	node := NewNode()
	//err := node.ConnectTCP("electrum1.bluewallet.io:443")
	//err := node.ConnectSSL("amd.kcalvinalvin.com:50002", &tls.Config{InsecureSkipVerify: true})
	err := node.ConnectSSL("127.0.0.1:50002", &tls.Config{InsecureSkipVerify: true})
	//err := node.ConnectSSL("mempool.space:60602", &tls.Config{InsecureSkipVerify: true})
	//err := node.ConnectSSL("signet-electrumx.wakiyamap.dev:50002", &tls.Config{InsecureSkipVerify: true})
	//err := node.ConnectSSL("electrum1.bluewallet.io:443", &tls.Config{InsecureSkipVerify: true})
	//err := node.ConnectSSL("electrum.emzy.de:50002", &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		fmt.Println("connect err", err)
		t.Fatal(err)
	}

	//address := "tb1pan8xuk638e5rpsx8yulm85zll2ux0etl2vtvkl9k36jkmmrhfm5s3u4mfq"
	address := "tb1qw9s6gs7j5vgy03f26c4yq9rgp3qw5gk8d5wttg"
	//address := "tb1q5lg99csv4lwp28e00873dmy92lrw8saw85z9wl"
	//address := "tb1qgml3mt8mht7fk6p0fdq3a3w4z4a36p5p4rwgaj"
	//address := "tb1pchrucyuknkgmqyutqfcae9hgquzmazz26gcw5yz3zvjsgdt3g7vsqrr2jl"
	//address := "bc1qwq4qkkfuxyrhdvsvzccheee7vzrwugcvdhhmns"
	//address := "bc1qp2uxwn9h7pzx5d828f34s295h3eefnuuka4dth"
	//address := "bc1qhl5ez6a3srxs2djdpeneasrv363udyyn4vggdt"
	//address := "3C5YrRZx6endtWkAGJJwVrHA8dDe6nUgS3"
	//address := "bc1qhcgj53gwgzs4mn4sted06f38zuwhkq2u9qre43"
	//address := "tb1qsph6alyup8se43pyk08r0e2pncar8wx86awurv"
	//address := "tb1q205t4um620gj26ru326n6rjfeuf6zh687wj464"
	//address := "tb1qzr279638gka0efh6vgvtjas076cwp25lej8udh"
	//address := "tb1qhcz8fy9xxkrgpkth42tncfuzzr4mnwmad05u5x"
	//address := "tb1q205t4um620gj26ru326n6rjfeuf6zh687wj464"
	//address := "tb1qnjheuvrfh0gmwc720z4yt97d6jmykr29u0yq9s"
	addr, err := btcutil.DecodeAddress(address, &chaincfg.SigNetParams)
	if err != nil {
		t.Fatal(err)
	}
	pkScript, err := txscript.PayToAddrScript(addr)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(pkScript)
	// Reverse the hash.
	sort.SliceStable(hash[:], func(i, j int) bool {
		return i > j
	})
	fmt.Println("scripthash ", hex.EncodeToString(hash[:]))
	hist, err := node.BlockchainScriptHashGetHistory(hex.EncodeToString(hash[:]))
	if err != nil {
		fmt.Println("err", err)
		t.Fatal(err)
	}

	for _, his := range hist {
		fmt.Printf("height %d, txid: %s\n", his.Height, his.TxHash)

		str := fmt.Sprintf("%s:%d:", his.TxHash, his.Height)

		var hash []byte
		chainHash := sha256.Sum256([]byte(str))
		hash = chainHash[:]

		fmt.Println("scripthash: ", hex.EncodeToString(hash))
	}
}

//func TestBlockchainScriptHashGetHistory1(t *testing.T) {
//	node := NewNode()
//	//err := node.ConnectTCP("electrum1.bluewallet.io:443")
//	//err := node.ConnectSSL("amd.kcalvinalvin.com:50002", &tls.Config{InsecureSkipVerify: true})
//	//err := node.ConnectTCP("127.0.0.1:50001")
//	//err := node.ConnectSSL("signet-electrumx.wakiyamap.dev:50002", &tls.Config{InsecureSkipVerify: true})
//	//err := node.ConnectSSL("electrum1.bluewallet.io:443", &tls.Config{InsecureSkipVerify: true})
//	err := node.ConnectSSL("electrum.emzy.de:50002", &tls.Config{InsecureSkipVerify: true})
//	if err != nil {
//		fmt.Println("connect err", err)
//		t.Fatal(err)
//	}
//
//	pkScript, err := hex.DecodeString("514104cc71eb30d653c0c3163990c47b976f3fb3f37cccdcbedb169a1dfef58bbfbfaff7d8a473e7e2e6d317b87bafe8bde97e3cf8f065dec022b51d11fcdd0d348ac4410461cbdcc5409fb4b4d42b51d33381354d80e550078cb532a34bfa2fcfdeb7d76519aecc62770f5b0e4ef8551946d8a540911abe3e7854a26f39f58b25c15342af52ae")
//	if err != nil {
//		t.Fatal(err)
//	}
//	hash := sha256.Sum256(pkScript)
//	// Reverse the hash.
//	sort.SliceStable(hash[:], func(i, j int) bool {
//		return i > j
//	})
//	hist, err := node.BlockchainScriptHashGetHistory(hex.EncodeToString(hash[:]))
//	if err != nil {
//		fmt.Println("err", err)
//		t.Fatal(err)
//	}
//
//	for _, his := range hist {
//		fmt.Printf("height %d, txid: %s\n", his.Height, his.TxHash)
//
//		str := fmt.Sprintf("%s:%d:", his.TxHash, his.Height)
//
//		var hash []byte
//		chainHash := sha256.Sum256([]byte(str))
//		hash = chainHash[:]
//
//		fmt.Println("scripthash: ", hex.EncodeToString(hash))
//	}
//}

func TestBlockchainScriptHashListUnspent(t *testing.T) {
	node := NewNode()
	//err := node.ConnectTCP("electrum1.bluewallet.io:443")
	//err := node.ConnectSSL("amd.kcalvinalvin.com:50002", &tls.Config{InsecureSkipVerify: true})
	//err := node.ConnectTCP("127.0.0.1:50001")
	//err := node.ConnectSSL("signet-electrumx.wakiyamap.dev:50002", &tls.Config{InsecureSkipVerify: true})
	//err := node.ConnectSSL("electrum1.bluewallet.io:443", &tls.Config{InsecureSkipVerify: true})
	err := node.ConnectSSL("electrum.emzy.de:50002", &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		fmt.Println("connect err", err)
		t.Fatal(err)
	}

	address := "1BdywN8HyrX87gyQKuKVkA9qjNKoThiXti"
	//address := "bc1qhl5ez6a3srxs2djdpeneasrv363udyyn4vggdt"
	//address := "3C5YrRZx6endtWkAGJJwVrHA8dDe6nUgS3"
	//address := "bc1qhcgj53gwgzs4mn4sted06f38zuwhkq2u9qre43"
	//address := "tb1qsph6alyup8se43pyk08r0e2pncar8wx86awurv"
	//address := "tb1q205t4um620gj26ru326n6rjfeuf6zh687wj464"
	//address := "tb1qzr279638gka0efh6vgvtjas076cwp25lej8udh"
	//address := "tb1qhcz8fy9xxkrgpkth42tncfuzzr4mnwmad05u5x"
	//address := "tb1q205t4um620gj26ru326n6rjfeuf6zh687wj464"
	//address := "tb1qnjheuvrfh0gmwc720z4yt97d6jmykr29u0yq9s"
	addr, err := btcutil.DecodeAddress(address, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatal(err)
	}
	pkScript, err := txscript.PayToAddrScript(addr)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(pkScript)
	// Reverse the hash.
	sort.SliceStable(hash[:], func(i, j int) bool {
		return i > j
	})
	list, err := node.BlockchainAddressListUnspent(hex.EncodeToString(hash[:]))
	if err != nil {
		fmt.Println("err", err)
		t.Fatal(err)
	}

	for _, elem := range list {
		fmt.Printf("height %d, txid: %s, value %d, pos %d\n",
			elem.Height, elem.Hash, elem.Value, elem.Pos)

		str := fmt.Sprintf("%s:%d:", elem.Hash, elem.Height)

		var hash []byte
		chainHash := sha256.Sum256([]byte(str))
		hash = chainHash[:]

		fmt.Println("scripthash: ", hex.EncodeToString(hash))
	}
}

func TestHash(t *testing.T) {
	//hashStr := "76a91462e907b15cbf27d5425399ebf6f0fb50ebb88f1888ac"
	//hashStr := "0014d18cf11dc93b63951b7dd5a7425dcdc0beb8bc3e"

	//hashStr := "0014b67401249cbb9969b0c18c76d7121b4f5581bfb9"
	//hashStr := "0014b67401249cbb9969b0c18c76d7121b4f5581bfb9"
	hashStr := "00146aacbc0c21db0bbb1d7da60157dc50f1cfdaff0d"
	fmt.Println("script:", hashStr)
	hash, err := hex.DecodeString(hashStr)
	if err != nil {
		fmt.Println(err)
	}
	sum := sha256.Sum256(hash)
	fmt.Printf("hashed script: %s\n", hex.EncodeToString(sum[:]))

	reversedHash := new(chainhash.Hash)
	err = chainhash.Decode(reversedHash, hex.EncodeToString(sum[:]))
	if err != nil {
		fmt.Println(err)
	}

	fmt.Printf("reversed: %s\n", hex.EncodeToString(reversedHash[:]))
}

func TestBlockchainHeader(t *testing.T) {
	node := NewNode()
	//err := node.ConnectSSL("signet-electrumx.wakiyamap.dev:50002", &tls.Config{InsecureSkipVerify: true})
	err := node.ConnectTCP("127.0.0.1:50001")
	if err != nil {
		fmt.Println("connect err", err)
		t.Fatal(err)
	}

	header, err := node.BlockchainGetBlockHeader(10000)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println("header: ", header)
}

func TestBlockchainHeaders(t *testing.T) {
	node := NewNode()
	err := node.ConnectSSL("signet-electrumx.wakiyamap.dev:50002", &tls.Config{InsecureSkipVerify: true})
	//err := node.ConnectTCP("127.0.0.1:50001")
	if err != nil {
		fmt.Println("connect err", err)
		t.Fatal(err)
	}

	headers, err := node.BlockchainGetBlockHeaders(1, 2)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println("header: ", headers)
}

func TestBlockchainHeadersSubscribe(t *testing.T) {
	node := NewNode()
	//err := node.ConnectSSL("electrum.emzy.de:50002", &tls.Config{InsecureSkipVerify: true})
	//err := node.ConnectSSL("mempool.space:60602", &tls.Config{InsecureSkipVerify: true})
	err := node.ConnectSSL("127.0.0.1:50002", &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		fmt.Println("connect err", err)
		t.Fatal(err)
	}

	headersChan, _ := node.BlockchainHeadersSubscribe()
	for {
		select {
		case str := <-headersChan:
			fmt.Println("header: ", str)
		}
	}
}

func TestBlockchainScriptHashSubscribe(t *testing.T) {
	node := NewNode()
	//err := node.ConnectSSL("electrum.emzy.de:50002", &tls.Config{InsecureSkipVerify: true})
	//err := node.ConnectTCP("127.0.0.1:50001")
	//err := node.ConnectSSL("signet-electrumx.wakiyamap.dev:50002", &tls.Config{InsecureSkipVerify: true})
	//err := node.ConnectSSL("127.0.0.1:50002", &tls.Config{InsecureSkipVerify: true})
	err := node.ConnectSSL("mempool.space:60602", &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		fmt.Println("connect err", err)
		t.Fatal(err)
	}

	//address := "bc1qltlt92dp89ph870hwvz6ksmhz982zqeh7vvljq"
	//address := "bc1qhl5ez6a3srxs2djdpeneasrv363udyyn4vggdt"
	//address := "bc1qdrv5l9a4mn9gd2yvzxkfftfv2s6j75vtu3w4yt"
	//address := "bcrt1qduc2gmuwkun9wnlcfp6ak8zzphmyee4dakgnlk"
	//address := "tb1q205t4um620gj26ru326n6rjfeuf6zh687wj464"
	//address := "tb1qhcz8fy9xxkrgpkth42tncfuzzr4mnwmad05u5x"
	//address := "tb1qah6avxhn65tup39xjx36ety3lcdwunffan4tdc"
	//address := "tb1q2m2napd6uktall8y6hgxre77ayxfqkzc80nsss"
	//address := "tb1q0cy0tt4spfdlsca80latrfgldsp85pj8rujtt2"
	//address := "bc1qwq4qkkfuxyrhdvsvzccheee7vzrwugcvdhhmns"
	//address := "bc1qp2uxwn9h7pzx5d828f34s295h3eefnuuka4dth"
	//address := "tb1qgml3mt8mht7fk6p0fdq3a3w4z4a36p5p4rwgaj"
	//address := "tb1pchrucyuknkgmqyutqfcae9hgquzmazz26gcw5yz3zvjsgdt3g7vsqrr2jl"
	//address := "tb1qw76rke3uk6a4m9l45uzjluq0q6r2xvuypu8mma"
	//address := "tb1q54jqr22qx2f7nc9sac9jhhqnnw0w4p5ajantew"
	//address := "tb1plh35dglv6w0q3vp29pm95nuy32uj58htfyn67yd2quwyjawykmcqu5szxp"
	address := "tb1qw9s6gs7j5vgy03f26c4yq9rgp3qw5gk8d5wttg"
	statusChan, err := node.BlockchainScriptHashSubscribe(address)
	if err != nil {
		fmt.Println("err", err)
		t.Fatal(err)
	}
	for {
		select {
		case str := <-statusChan:
			fmt.Println("status: ", str)
		}
	}
}

func TestBlockchainScriptHashSubscribe1(t *testing.T) {
	node := NewNode()
	//err := node.ConnectSSL("electrum.emzy.de:50002", &tls.Config{InsecureSkipVerify: true})
	//err := node.ConnectTCP("127.0.0.1:50001")
	//err := node.ConnectSSL("signet-electrumx.wakiyamap.dev:50002", &tls.Config{InsecureSkipVerify: true})
	err := node.ConnectSSL("127.0.0.1:50002", &tls.Config{InsecureSkipVerify: true})
	//err := node.ConnectSSL("mempool.space:60602", &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		fmt.Println("connect err", err)
		t.Fatal(err)
	}

	//address := "bc1qltlt92dp89ph870hwvz6ksmhz982zqeh7vvljq"
	//address := "bc1qhl5ez6a3srxs2djdpeneasrv363udyyn4vggdt"
	//address := "bc1qdrv5l9a4mn9gd2yvzxkfftfv2s6j75vtu3w4yt"
	//address := "bcrt1qduc2gmuwkun9wnlcfp6ak8zzphmyee4dakgnlk"
	//address := "tb1q205t4um620gj26ru326n6rjfeuf6zh687wj464"
	//address := "tb1qhcz8fy9xxkrgpkth42tncfuzzr4mnwmad05u5x"
	//address := "tb1qah6avxhn65tup39xjx36ety3lcdwunffan4tdc"
	//address := "tb1q2m2napd6uktall8y6hgxre77ayxfqkzc80nsss"
	//address := "tb1q0cy0tt4spfdlsca80latrfgldsp85pj8rujtt2"
	//address := "bc1qwq4qkkfuxyrhdvsvzccheee7vzrwugcvdhhmns"
	//address := "bc1qp2uxwn9h7pzx5d828f34s295h3eefnuuka4dth"
	//address := "tb1qgml3mt8mht7fk6p0fdq3a3w4z4a36p5p4rwgaj"
	//address := "tb1pchrucyuknkgmqyutqfcae9hgquzmazz26gcw5yz3zvjsgdt3g7vsqrr2jl"
	//address := "tb1qw76rke3uk6a4m9l45uzjluq0q6r2xvuypu8mma"
	//address := "tb1q54jqr22qx2f7nc9sac9jhhqnnw0w4p5ajantew"
	address := "tb1plh35dglv6w0q3vp29pm95nuy32uj58htfyn67yd2quwyjawykmcqu5szxp"
	statusChan, err := node.BlockchainScriptHashSubscribe(address)
	if err != nil {
		fmt.Println("err", err)
		t.Fatal(err)
	}
	for {
		select {
		case str := <-statusChan:
			fmt.Println("status: ", str)
		}
	}
}

func TestParams(t *testing.T) {
	address := "\"" + "tb1q4upesern5fy2u8mf8lznrqdw47zk2tphyj2wsk" + "\""
	asdf := "\"" + "jslsdfj" + "\""

	params := []string{address, asdf}
	params = []string{strings.Join(params, ",")}
	fmt.Println("params: ", params)

	bytes, _ := json.Marshal(params)
	fmt.Println("bytes: ", bytes)

	decodedParams := []string{}
	json.Unmarshal(bytes, &decodedParams)
	fmt.Println("decoded params", decodedParams)
}

func TestBlockchainAddressGetBalance(t *testing.T) {
	node := NewNode()
	//err := node.ConnectTCP("electrum1.bluewallet.io:443")
	//err := node.ConnectSSL("amd.kcalvinalvin.com:50002", &tls.Config{InsecureSkipVerify: true})
	err := node.ConnectTCP("127.0.0.1:50001")
	//err := node.ConnectSSL("electrum1.bluewallet.io:443", &tls.Config{InsecureSkipVerify: true})
	//err := node.ConnectSSL("electrum.emzy.de:50002", &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		fmt.Println("connect err", err)
		t.Fatal(err)
	}

	//address := "bc1qp2uxwn9h7pzx5d828f34s295h3eefnuuka4dth"
	//address := "bc1qz8skktdet3xlgyagnws9dsmj5lmghmmkkrs7aj"
	//address := "bc1qhl5ez6a3srxs2djdpeneasrv363udyyn4vggdt"
	//address := "tb1q4upesern5fy2u8mf8lznrqdw47zk2tphyj2wsk"
	address := "bcrt1qduc2gmuwkun9wnlcfp6ak8zzphmyee4dakgnlk"
	//address := "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa"
	balance, err := node.BlockchainAddressGetBalance(address)
	if err != nil {
		fmt.Println("err", err)
		t.Fatal(err)
	}
	fmt.Println("balance", balance)
}

func TestDonationAddress(t *testing.T) {
	//node := NewNode()
	////err := node.ConnectSSL("amd.kcalvinalvin.com:50002", &tls.Config{InsecureSkipVerify: true})
	////err := node.ConnectSSL("electrum.acinq.co:50002", &tls.Config{InsecureSkipVerify: true})
	////err := node.ConnectSSL("electrum1.bluewallet.io:443", &tls.Config{InsecureSkipVerify: true})
	//err := node.ConnectTCP("127.0.0.1:50001")
	////err := node.ConnectSSL("electrum.emzy.de:50002", &tls.Config{InsecureSkipVerify: true})
	//if err != nil {
	//	fmt.Println("connect err", err)
	//	t.Fatal(err)
	//}
	//donationAddr, err := node.ServerDonationAddress()
	//if err != nil {
	//	fmt.Println("err", err)
	//	t.Fatal(err)
	//}
	//fmt.Println("donationAddr", donationAddr)

	str := "1e0aa4ac97dae3b1698b5b924780bd73ee6a7d2750bff4ee89892d6f7b53f871"
	fmt.Println("str:", str)
	decodedHash := new(chainhash.Hash)
	err := chainhash.Decode(decodedHash, str)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Printf("decodedhash: %s\n", hex.EncodeToString(decodedHash[:]))

	newHash, _ := chainhash.NewHash(decodedHash[:])
	fmt.Printf("newhash: %s\n", newHash.String())

	sum := sha256.Sum256(decodedHash[:])
	fmt.Printf("hashed script: %s\n", hex.EncodeToString(sum[:]))

	//hashStr := "76a91462e907b15cbf27d5425399ebf6f0fb50ebb88f1888ac"
	//hashStr := "2462c0b1d4a6163041015e1c7db4708c63147f87"
	//fmt.Println("script:", hashStr)
	//hash, err := hex.DecodeString(hashStr)
	//if err != nil {
	//	fmt.Println(err)
	//}
	//sum := sha256.Sum256(hash)
	//fmt.Printf("hashed script: %s\n", hex.EncodeToString(sum[:]))

	//str = "3716f52f0e71620b6c92070dbb9af05bd4ad7e421b4f0c5f8ee08154c929d4b1:774659:"
	//sum = sha256.Sum256([]byte(str))
	//fmt.Printf("hash of decoded string: %s\n", hex.EncodeToString(sum[:]))
}

//	func TestServerVersion(t *testing.T) {
//		node := NewNode()
//		//err := node.ConnectSSL("electrum1.bluewallet.io:443", &tls.Config{InsecureSkipVerify: true})
//		err := node.ConnectSSL("electrum.acinq.co:50002", &tls.Config{InsecureSkipVerify: true})
//		if err != nil {
//			t.Fatal(err)
//		}
//
//		version, err := node.ServerVersion()
//		if err != nil {
//			t.Fatal(err)
//		}
//		fmt.Println("version", version)
//	}

func TestServerFeatures(t *testing.T) {
	node := NewNode()
	//err := node.ConnectSSL("electrum1.bluewallet.io:443", &tls.Config{InsecureSkipVerify: true})
	//err := node.ConnectSSL("amd.kcalvinalvin.com:50002", &tls.Config{InsecureSkipVerify: true})
	//err := node.ConnectSSL("electrum.emzy.de:50002", &tls.Config{InsecureSkipVerify: true})
	err := node.ConnectTCP("127.0.0.1:50001")
	if err != nil {
		t.Fatal(err)
	}

	features, err := node.ServerFeatures()
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("features", features)
}

func TestGetTransaction(t *testing.T) {
	node := NewNode()
	err := node.ConnectTCP("127.0.0.1:50001")
	if err != nil {
		fmt.Println("connect err", err)
		t.Fatal(err)
	}
	txid := "5a51025ac86f3580891488e15fbbb797d56efe46f6da6170f9fc6cd67caad36b"

	rawTx, err := node.BlockchainTransactionGet(txid)
	if err != nil {
		fmt.Println("connect err", err)
		t.Fatal(err)
	}

	fmt.Println("rawTx: ", rawTx)
}

func TestBlockchainGetBlockHeader(t *testing.T) {
	node := NewNode()
	//err := node.ConnectSSL("electrum1.bluewallet.io:443", &tls.Config{InsecureSkipVerify: true})
	//if err != nil {
	//	fmt.Println("connect err", err)
	//	t.Fatal(err)
	//}

	//err := node.ConnectTCP("signet.nunchuk.io:50002")
	//if err != nil {
	//	fmt.Println("connect err", err)
	//	t.Fatal(err)
	//}

	err := node.ConnectTCP("127.0.0.1:50001")
	if err != nil {
		fmt.Println("connect err", err)
		t.Fatal(err)
	}

	headerRes, err := node.BlockchainGetBlockHeader(8)
	if err != nil {
		fmt.Println("err", err)
		t.Fatal(err)
	}
	fmt.Println("headerRes", headerRes)
	//hash := new(chainhash.Hash)
	//err = chainhash.Decode(hash, headerRes)
	//if err != nil {
	//	fmt.Println("err", err)
	//	t.Fatal(err)
	//}
	//bytes, err := hex.DecodeString(headerRes)
	//if err != nil {
	//	fmt.Println("err", err)
	//	t.Fatal(err)
	//}
	//fmt.Printf("headerRes %x\n", sha256.Sum256(bytes))
}

func TestPing(t *testing.T) {
	node := NewNode()
	//err := node.ConnectSSL("electrum1.bluewallet.io:443", &tls.Config{InsecureSkipVerify: true})
	//err := node.ConnectSSL("amd.kcalvinalvin.com:50002", &tls.Config{InsecureSkipVerify: true})
	//err := node.ConnectSSL("electrum.emzy.de:50002", &tls.Config{InsecureSkipVerify: true})
	err := node.ConnectTCP("127.0.0.1:50001")
	if err != nil {
		t.Fatal(err)
	}

	node.Ping()
}

func TestBlockchainEstimateFee(t *testing.T) {
	node := NewNode()
	err := node.ConnectTCP("127.0.0.1:50001")
	if err != nil {
		fmt.Println("connect err", err)
		t.Fatal(err)
	}

	fee, err := node.BlockchainEstimateFee(2)
	if err != nil {
		fmt.Println("err", err)
		t.Fatal(err)
	}
	fmt.Println("fee", fee)
}

func TestTransactionGetMerkle(t *testing.T) {
	node := NewNode()
	err := node.ConnectTCP("127.0.0.1:50001")
	//err := node.ConnectSSL("electrum.emzy.de:50002", &tls.Config{InsecureSkipVerify: true})
	//err := node.ConnectSSL("signet-electrumx.wakiyamap.dev:50002", &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		fmt.Println("connect err", err)
		t.Fatal(err)
	}

	txHash, err := chainhash.NewHashFromStr("2f1dd8e23cda11b408a33ecfe9eda7eaba53a4645df527166265f73ddeb42af2")
	if err != nil {
		t.Fatal(err)
	}
	getMerkle, err := node.BlockchainTransactionGetMerkle(*txHash, 133105)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println(getMerkle)
}

//func TestBlockchainEstimateFee(t *testing.T) {
//	//node := NewNode()
//	////err := node.ConnectTCP("electrum1.bluewallet.io:443")
//	////err := node.ConnectSSL("amd.kcalvinalvin.com:50002", &tls.Config{InsecureSkipVerify: true})
//	////err := node.ConnectTCP("127.0.0.1:50001")
//	////err := node.ConnectSSL("electrum1.bluewallet.io:443", &tls.Config{InsecureSkipVerify: true})
//	//err := node.ConnectSSL("electrum.emzy.de:50002", &tls.Config{InsecureSkipVerify: true})
//	//if err != nil {
//	//	fmt.Println("connect err", err)
//	//	t.Fatal(err)
//	//}
//
//	//features, err := node.ServerFeatures()
//	//if err != nil {
//	//	fmt.Println("err", err)
//	//	t.Fatal(err)
//	//}
//	//fmt.Println("features", features)
//
//	//headerRes, err := node.BlockchainGetBlockHeader(70000)
//	//if err != nil {
//	//	fmt.Println("err", err)
//	//	t.Fatal(err)
//	//}
//	//fmt.Println("headerRes", headerRes)
//
//	addr, err := btcutil.DecodeAddress("1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa", &chaincfg.MainNetParams)
//	if err != nil {
//		fmt.Println("err", err)
//		t.Fatal(err)
//	}
//
//	pkScript, err := txscript.PayToAddrScript(addr)
//	if err != nil {
//		fmt.Println("err", err)
//		t.Fatal(err)
//	}
//	fmt.Println("pkscript", hex.EncodeToString(pkScript))
//	hash := sha256.Sum256(pkScript)
//	fmt.Println("pkscript sha256 hash", hex.EncodeToString(hash[:]))
//	fmt.Println("pkscript sha256 hash", fmt.Sprintf("%x", hash[:]))
//
//	fmt.Println("addr", hex.EncodeToString(addr.ScriptAddress()))
//	fmt.Println("addr", addr.String())
//	fmt.Println("addr", addr.EncodeAddress())
//
//	//balance, err := node.BlockchainAddressGetBalance("14a2y9Ut8JwW8DcTdZMaa31d5JoQQEQGbV")
//	//if err != nil {
//	//	fmt.Println("err", err)
//	//	t.Fatal(err)
//	//}
//	//fmt.Println("balance", balance)
//
//	//donationAddr, err := node.ServerDonationAddress()
//	//if err != nil {
//	//	fmt.Println("err", err)
//	//	t.Fatal(err)
//	//}
//	//fmt.Println("donationAddr", donationAddr)
//
//	//err = node.Ping()
//	//if err != nil {
//	//	fmt.Println("err", err)
//	//	t.Fatal(err)
//	//}
//
//	//fmt.Println("ping success")
//
//	//version, err := node.ServerVersion()
//	//if err != nil {
//	//	fmt.Println("err", err)
//	//	t.Fatal(err)
//	//}
//	//fmt.Println("version", version)
//
//	//fee, err := node.BlockchainEstimateFee(2)
//	//if err != nil {
//	//	fmt.Println("err", err)
//	//	t.Fatal(err)
//	//}
//	//fmt.Println("fee", fee)
//}

func TestMempoolGetFeeHistogram(t *testing.T) {
	node := NewNode()
	err := node.ConnectTCP("127.0.0.1:50001")
	//err := node.ConnectSSL("electrum.emzy.de:50002", &tls.Config{InsecureSkipVerify: true})
	//err := node.ConnectTCP("electrum.bitaroo.net:50002")
	//err := node.ConnectSSL("signet-electrumx.wakiyamap.dev:50002", &tls.Config{InsecureSkipVerify: true})
	//err := node.ConnectTCP("electrum1.bluewallet.io:443")
	if err != nil {
		fmt.Println("connect err", err)
		t.Fatal(err)
	}

	histogram, err := node.MempoolGetFeeHistogram()
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println("histogram: ", histogram)
}
