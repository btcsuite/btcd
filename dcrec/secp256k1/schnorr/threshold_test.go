// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package schnorr

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"math/rand"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrec/secp256k1"
)

type signerHex struct {
	privkey          string
	privateNonce     string
	pubKeySumLocal   string
	partialSignature string
}

// Sha256. The internal tests from secp256k1 are kind of screwy and for
// partial signatures call this hash function instead of testSchnorrHash.
func testSchnorrSha256Hash(msg []byte) []byte {
	sha := sha256.Sum256(msg)
	return sha[:]
}

type ThresholdTestVectorHex struct {
	msg               string
	signersHex        []signerHex
	combinedSignature string
}

// thresholdTestVectorsHex were produced using the testing functions
// implemented in libsecp256k1.
// https://github.com/bitcoin/secp256k1/blob/258720851e24e23c1036b4802a185850e258a105/src/modules/schnorr/tests_impl.h
var thresholdTestVectorsHex = []*ThresholdTestVectorHex{
	{
		msg: "0000000000F8FFFFFFFFFF0000000000F0FF3F0000000000C0FFFFFFFF000000",
		signersHex: []signerHex{
			{
				privkey:          "FFFFFFFFFFFFFF0300007800000000000000000000000000000000C0FF010000",
				privateNonce:     "2C36EFC7F20528A4BA3EF4908786A68B1C5CE4C8BD583F430EBC7E0D8DA853E1",
				pubKeySumLocal:   "031A763FD2C84354C4BD2B63487952A34DD0E2B9D1E44CA226DC7BAD086D40C29F",
				partialSignature: "22E0613D5D406CBAE5B5EFBC39B974E95EFC0DAEF946469F5F68B8E416F398D3B741EED58262E511A6D4BBAD386D6F344AEC2E6A383DA991384F7083B1CE5393",
			},
			{
				privkey:          "FFFFFFFFFFFFFF0F00000000F8FF0700F0FFFFFFFFFF0300FEFFFFFFFF070000",
				privateNonce:     "52347F775C44520F7F3E60ADDAA4E1CBA89E2BE3A82C89B9E11556B081A3D3EF",
				pubKeySumLocal:   "037C1D54CF0E2C3947AC9D0C046325A441D2C49FE866423E7FE2A088B2AAA57BC6",
				partialSignature: "22E0613D5D406CBAE5B5EFBC39B974E95EFC0DAEF946469F5F68B8E416F398D3D6BB539760D9C5965204FEB2F2399D50242E112C74D11642B2468AA82245A178",
			},
			{
				privkey:          "0000000000000000000000FEFFFFFF7F00F8FFFFFFFFFF0F000080FF0FE0FFFF",
				privateNonce:     "6F3E98F7B477BAD4D2F7172925EC495BDF1E80A7D4D6E67C180D8577E7D71E9C",
				pubKeySumLocal:   "033C9E82854DCA364A32295BCA2C8C3CC19633FDB1A51D5E1C6B41A3E19E8468B3",
				partialSignature: "22E0613D5D406CBAE5B5EFBC39B974E95EFC0DAEF946469F5F68B8E416F398D36FB69AE37DDBF2CCB254CE6D936D285E549A1225AF267AED8F121C84BC68CA29",
			},
		},
		combinedSignature: "22E0613D5D406CBAE5B5EFBC39B974E95EFC0DAEF946469F5F68B8E416F398D3FDB3DD5061189D74AB2E88CDBE1434E4090574D5ACEC9A85B9D5B923C0467DF3",
	},
	{
		msg: "FFFFE30000F0FFFFFFFFFF0700000000000000000000040000000080FFFFFFFF",
		signersHex: []signerHex{
			{
				privkey:          "FFFFFFFFFFFFFFFFFFFFFFFF1F000000E0FFFFFFFFFFFF7F0000FFFFFFFFFFFF",
				privateNonce:     "A2880E3B8C3484EAC0B48D440C866480DB46EBD2E2171D9E72C2168BA090E882",
				pubKeySumLocal:   "022D3E998D4C45E497507F9181DB7638303607F70DCDDCF34AF6189BCA1E02B765",
				partialSignature: "1B7510DAF07385B967325B00C2D38C2E510DE7B7E4DD3FA21BFB3F9BDC67C8C63C527290CD1A19C9F84BDF57C5E241C15FAE84BB3A8C7394A2BF67CF4D5CD2F7",
			},
			{
				privkey:          "000000000000C0FFFF0100FFFFFFFFFFFFFFFF0000000000000000000000F8CF",
				privateNonce:     "5527AE1CF2355A01477A672F6538C8F45BBE0B24BD67C0C97B70EE68CF92D169",
				pubKeySumLocal:   "0391076906B5293F3061CF4C54E18241156B37CC49087C5FF8A2B898AD020C21C6",
				partialSignature: "1B7510DAF07385B967325B00C2D38C2E510DE7B7E4DD3FA21BFB3F9BDC67C8C60A34632DCE40BFDB7D63EFA0D615F75A6CC6A5E39DF84B5016F2CE59CD0EB0C8",
			},
			{
				privkey:          "0000F83F0000000000F03F00000000C0FFFFFF0700000000FEFFFFFFFFFF3F00",
				privateNonce:     "4CAF0B156FAA4C59B2C2B98C3ACDE8FF2ADDB9B0795A8D8C64F93C2230B4EE00",
				pubKeySumLocal:   "02756380BE40F3D244ACE2AA1385B0E9ECA2A69D4845FD4C3351C80B6D7404AE01",
				partialSignature: "1B7510DAF07385B967325B00C2D38C2E510DE7B7E4DD3FA21BFB3F9BDC67C8C6025C67BA496A41EE038A749FFBAF8B8D849AB5FD489A61EC7750AB2C3F36F6DC",
			},
			{
				privkey:          "0100F8FFFFFFFF0F000000000000FEFF0F00000000FC7F0000F8FFFFFFFFFFFF",
				privateNonce:     "EDC53C5EF7441CF9B415AC2D670EB4850AE9EC9066A4BF24CBFD91505E4C94B6",
				pubKeySumLocal:   "039B5600241B681D80C2A9E8C2E4ED72E96E90AE04C857F2B0D36AE110486395B8",
				partialSignature: "1B7510DAF07385B967325B00C2D38C2E510DE7B7E4DD3FA21BFB3F9BDC67C8C6B8C285954F28998E45065E8E97FB7021CEDD8AD112A9238331F34AB0559AC64A",
			},
			{
				privkey:          "00000080030000000000000000E0FFFFFFFFFFFFFFFFFEFF0700000000FCFFFF",
				privateNonce:     "BD7014C0A5CD2ED2324994BE3235B12A18359BE7D4A9817D10EEDEB84E79EA25",
				pubKeySumLocal:   "0349DB0A59FE3B1D45CE29AE43C3E2D52DFF2A7854284F028C166E861321DC5ACE",
				partialSignature: "1B7510DAF07385B967325B00C2D38C2E510DE7B7E4DD3FA21BFB3F9BDC67C8C63DF61C0D15838D8E45ECD5E304C70DA5F34726F61D48CE74EEFD671578945F0B",
			},
		},
		combinedSignature: "1B7510DAF07385B967325B00C2D38C2E510DE7B7E4DD3FA21BFB3F9BDC67C8C63F9BDF1B497142B0042D780A346A42725885B57CA1C8728D9221348E579B5EAF",
	},
	{
		msg: "E63D5EE254ECC86A8BD6439E6434148BD02823E678B96816B08581EB33D2192A",
		signersHex: []signerHex{
			{
				privkey:          "7A39B587A29317AFD793F701D26542E3081D93AB58EA28968C491D7E066A1798",
				privateNonce:     "FA1F251871E84908FFCDAB664BB5BCBD81C6E99492A01D087CE251E70749A628",
				pubKeySumLocal:   "038054B95E8008E20BB4217DE652DB2483428851A01D4DB2E8012A69F1C665465F",
				partialSignature: "3AB82171C8668D549A1D41B8AB191B862891F06785FD2AEB01C314A0393F42F4A641D545821A52F7C8C2A3E4C8A142672AE5B912EA47C7146A90A0C796A32F94",
			},
			{
				privkey:          "A1FABBBA0C76DD5F255F1A1228D57ADBEBB935C6A80B2D877D3E153DBF946C33",
				privateNonce:     "C5BEF72D01E30C9D31D6B48A2582FE11E10383F99B933878C308F63718F7FF19",
				pubKeySumLocal:   "02A4046C2D277BE14A6DF6A9B7664CBB075A3F4B68C2E8D64B9F71A495132D9E65",
				partialSignature: "3AB82171C8668D549A1D41B8AB191B862891F06785FD2AEB01C314A0393F42F4B7036345F99BD18852418A3FAF40A3E89B61CA933795282B89D1026CEE88E0E2",
			},
			{
				privkey:          "41BB6B56F127607218B6C0D4DDC9595A6639172ECAFBB73134B16EF6852627DD",
				privateNonce:     "07B4D13FFD4DC040AEDEC4FA41584F8DC30E01DA86F1D945A787FCD3AEEC2231",
				pubKeySumLocal:   "0221FF2EC05F57519E33A035B9566E2E7B14CB5E28C4AC6E395E5E2F22A2DD0B69",
				partialSignature: "3AB82171C8668D549A1D41B8AB191B862891F06785FD2AEB01C314A0393F42F47BEF592BC2C18D90ED2CCEF245ED861443C71F54913794913A614402E7CA0099",
			},
		},
		combinedSignature: "3AB82171C8668D549A1D41B8AB191B862891F06785FD2AEB01C314A0393F42F4D93491B73E77B2110830FD16BDCF6C654F5FC61403CBE3956EF088AA9CBFCFCE",
	},
	{
		msg: "07BE073995BF78D440B660AF7B06DC0E9BA120A8D686201989BA99AA384ADF12",
		signersHex: []signerHex{
			{
				privkey:          "23527007EB04CA9E0789C3F452A0B154338FCDEF1FF8C88C2FD52066E3684E52",
				privateNonce:     "EFB2ED7B375587DAEBBE8442768402CAA5F841F6342CA5D27C5F7BD7726D5192",
				pubKeySumLocal:   "027C8707533D31CD171EA0EF51616B45AE62BCF208DD54708D6A6FFA2AEA6527C5",
				partialSignature: "3D13F453F0C41503C00BEA5D558BB267E3E886C588A8A5BABCEED8A33839BEC38A1E913B96EFD3B6155A6B14913819472E2FFE710C51B94A668A6D202CC1D68A",
			},
			{
				privkey:          "CB7ADC7097149F4A803D06F772DB7787BCCAFEB2EB9B83BCD87F8E1DB7C7DEB4",
				privateNonce:     "3C13E81FAB1BF65B86309CADD2696BFB6155C36D193715D78F199AA0FFC856A9",
				pubKeySumLocal:   "02A3FD12EEFD04192106C3609CB5C9FA6C6DFDB2380236BE0A9440563F4018C473",
				partialSignature: "3D13F453F0C41503C00BEA5D558BB267E3E886C588A8A5BABCEED8A33839BEC39E30A3A4B6B95CD0F3650961A75E00912B4B1A5D504027C60674C7B347A74FEB",
			},
			{
				privkey:          "13F64737DB01990CFB23DCF911EE9544821367BABF250E8D14278518523AE2EE",
				privateNonce:     "2EAFD6A829C8174E28673D42299A4E7BAD51B736A57DEC3E654B48BF7530720E",
				pubKeySumLocal:   "027851635454EDA9BFDF6405122C9E71E1FB0F3D8B460AD77E30576DC33F46DB7F",
				partialSignature: "3D13F453F0C41503C00BEA5D558BB267E3E886C588A8A5BABCEED8A33839BEC39BA4523516A1DE4049B20ACC0FF5335BBC2769A0EE2C2A90B85DAEEDC5DC5534",
			},
			{
				privkey:          "F969F8FFA6320EFE7450AFDED4802DBB65EC0FC634831282898A9E1A148A672D",
				privateNonce:     "97C3B4FEB72BF8045943A5AD0A994E61A71EFFFBDEECC9F82C47A33FAAD545AA",
				pubKeySumLocal:   "03879186ED812C4106644FCB1E47951788799566487C7A1996AB6B226ABCD44361",
				partialSignature: "3D13F453F0C41503C00BEA5D558BB267E3E886C588A8A5BABCEED8A33839BEC3C03E18DDC9EE7181341D9B720F660F445E7900A5D6A156A237DC45F99D69A239",
			},
			{
				privkey:          "7290D1CE9CED9EA92826BB930746B62C0D6476233778A25B46602B2CDD22521C",
				privateNonce:     "B6E877016CFDE7F49324D7270B2617B7E6BAE84EB674667143799DB5474A8EE0",
				pubKeySumLocal:   "0201EED57034444E89738D8E91D969DB9D46C08ADE5E8F5E135A3510881ACBEC5B",
				partialSignature: "3D13F453F0C41503C00BEA5D558BB267E3E886C588A8A5BABCEED8A33839BEC3C0FB656897A0401B9118D3C243E923664E5C33818386841D0729C4552B2D0285",
			},
		},
		combinedSignature: "3D13F453F0C41503C00BEA5D558BB267E3E886C588A8A5BABCEED8A33839BEC3452D055BC5D9C06417A7EE769BDA7FE2926B1FE2970C05AD24EBD26992395CA4",
	},
}

type signer struct {
	privkey          []byte
	pubkey           *secp256k1.PublicKey
	privateNonce     []byte
	publicNonce      *secp256k1.PublicKey
	pubKeySumLocal   *secp256k1.PublicKey
	partialSignature []byte
}

type ThresholdTestVector struct {
	msg               []byte
	signers           []signer
	combinedSignature []byte
}

func GetThresholdTestVectors() []*ThresholdTestVector {
	var tvs []*ThresholdTestVector
	for _, v := range thresholdTestVectorsHex {
		msg, _ := hex.DecodeString(v.msg)
		combSig, _ := hex.DecodeString(v.combinedSignature)
		signers := make([]signer, len(v.signersHex))
		for i, signerHex := range v.signersHex {
			privkeyB, _ := hex.DecodeString(signerHex.privkey)
			_, pubkey := secp256k1.PrivKeyFromBytes(privkeyB)
			privateNonceB, _ := hex.DecodeString(signerHex.privateNonce)
			_, noncePub := secp256k1.PrivKeyFromBytes(privateNonceB)
			pubKeySumLocalB, _ := hex.DecodeString(signerHex.pubKeySumLocal)
			pubKeySumLocal, _ := secp256k1.ParsePubKey(pubKeySumLocalB)
			partialSignature, _ := hex.DecodeString(signerHex.partialSignature)

			signers[i].privkey = privkeyB
			signers[i].pubkey = pubkey
			signers[i].privateNonce = privateNonceB
			signers[i].publicNonce = noncePub
			signers[i].pubKeySumLocal = pubKeySumLocal
			signers[i].partialSignature = partialSignature
		}

		lv := ThresholdTestVector{
			msg:               msg,
			signers:           signers,
			combinedSignature: combSig,
		}

		tvs = append(tvs, &lv)
	}

	return tvs
}

func TestSchnorrThresholdRef(t *testing.T) {
	curve := secp256k1.S256()
	tvs := GetThresholdTestVectors()
	for _, tv := range tvs {
		partialSignatures := make([]*Signature, len(tv.signers))

		// Ensure all the pubkey and nonce derivation is correct.
		for i, signer := range tv.signers {
			nonce := nonceRFC6979(signer.privkey, tv.msg, nil,
				Sha256VersionStringRFC6979)
			cmp := bytes.Equal(nonce[:], signer.privateNonce[:])
			if !cmp {
				t.Fatalf("expected %v, got %v", true, cmp)
			}

			_, pubkey := secp256k1.PrivKeyFromBytes(signer.privkey)
			cmp = bytes.Equal(pubkey.Serialize()[:], signer.pubkey.Serialize()[:])
			if !cmp {
				t.Fatalf("expected %v, got %v", true, cmp)
			}

			_, pubNonce := secp256k1.PrivKeyFromBytes(nonce)
			cmp = bytes.Equal(pubNonce.Serialize()[:], signer.publicNonce.Serialize()[:])
			if !cmp {
				t.Fatalf("expected %v, got %v", true, cmp)
			}

			// Calculate the public nonce sum.
			pubKeys := make([]*secp256k1.PublicKey, len(tv.signers)-1)

			itr := 0
			for _, signer := range tv.signers {
				if bytes.Equal(signer.publicNonce.Serialize(),
					tv.signers[i].publicNonce.Serialize()) {
					continue
				}
				pubKeys[itr] = signer.publicNonce
				itr++
			}
			publicNonceSum := CombinePubkeys(pubKeys)
			cmp = bytes.Equal(publicNonceSum.Serialize()[:], signer.pubKeySumLocal.Serialize()[:])
			if !cmp {
				t.Fatalf("expected %v, got %v", true, cmp)
			}

			sig, err := schnorrPartialSign(curve, tv.msg, signer.privkey, nonce,
				publicNonceSum, testSchnorrSha256Hash)
			if err != nil {
				t.Fatalf("unexpected error %s, ", err)
			}

			cmp = bytes.Equal(sig.Serialize()[:], signer.partialSignature[:])
			if !cmp {
				t.Fatalf("expected %v, got %v", true, cmp)
			}

			partialSignatures[i] = sig
		}

		// Combine signatures.
		combinedSignature, err := CombineSigs(curve, partialSignatures)
		if err != nil {
			t.Fatalf("unexpected error %s, ", err)
		}

		cmp := bytes.Equal(combinedSignature.Serialize()[:], tv.combinedSignature[:])
		if !cmp {
			t.Fatalf("expected %v, got %v", true, cmp)
		}

		// Combine pubkeys.
		allPubkeys := make([]*secp256k1.PublicKey, len(tv.signers))
		for i, signer := range tv.signers {
			allPubkeys[i] = signer.pubkey
		}
		allPksSum := CombinePubkeys(allPubkeys)

		// Verify the combined signature and public keys.
		ok, err := schnorrVerify(combinedSignature.Serialize(),
			allPksSum, tv.msg, testSchnorrSha256Hash)
		if err != nil {
			t.Fatalf("unexpected error %s, ", err)
		}

		if !ok {
			t.Fatalf("expected %v, got %v", true, ok)
		}
	}
}

func TestSchnorrThreshold(t *testing.T) {
	tRand := rand.New(rand.NewSource(54321))
	maxSignatories := 10
	numTests := 100
	numSignatories := maxSignatories * numTests

	curve := secp256k1.S256()
	msg, _ := hex.DecodeString(
		"07BE073995BF78D440B660AF7B06DC0E9BA120A8D686201989BA99AA384ADF12")
	privkeys := randPrivKeyList(numSignatories)

	for i := 0; i < numTests; i++ {
		numKeysForTest := tRand.Intn(maxSignatories-2) + 2
		keyIndex := i * maxSignatories
		keysToUse := make([]*secp256k1.PrivateKey, numKeysForTest)
		for j := 0; j < numKeysForTest; j++ {
			keysToUse[j] = privkeys[j+keyIndex]
		}
		pubKeysToUse := make([]*secp256k1.PublicKey, numKeysForTest)
		for j := 0; j < numKeysForTest; j++ {
			_, pubkey := secp256k1.PrivKeyFromBytes(keysToUse[j].Serialize())
			pubKeysToUse[j] = pubkey
		}
		privNoncesToUse := make([]*secp256k1.PrivateKey, numKeysForTest)
		pubNoncesToUse := make([]*secp256k1.PublicKey, numKeysForTest)
		for j := 0; j < numKeysForTest; j++ {
			nonce := nonceRFC6979(keysToUse[j].Serialize(), msg, nil,
				BlakeVersionStringRFC6979)
			privNonce, pubNonce := secp256k1.PrivKeyFromBytes(nonce)
			privNoncesToUse[j] = privNonce
			pubNoncesToUse[j] = pubNonce
		}

		partialSignatures := make([]*Signature, numKeysForTest)

		// Partial signature generation.
		for j := range keysToUse {
			thisPubNonce := pubNoncesToUse[j]
			localPubNonces := make([]*secp256k1.PublicKey, numKeysForTest-1)
			itr := 0
			for _, pubNonce := range pubNoncesToUse {
				if bytes.Equal(thisPubNonce.Serialize(), pubNonce.Serialize()) {
					continue
				}
				localPubNonces[itr] = pubNonce
				itr++
			}
			publicNonceSum := CombinePubkeys(localPubNonces)

			sig, err := schnorrPartialSign(curve, msg, keysToUse[j].Serialize(),
				privNoncesToUse[j].Serialize(), publicNonceSum,
				chainhash.HashB)
			if err != nil {
				t.Fatalf("unexpected error %s, ", err)
			}

			partialSignatures[j] = sig
		}

		// Combine signatures.
		combinedSignature, err := CombineSigs(curve, partialSignatures)
		if err != nil {
			t.Fatalf("unexpected error %s, ", err)
		}

		// Combine pubkeys.
		allPubkeys := make([]*secp256k1.PublicKey, numKeysForTest)
		copy(allPubkeys, pubKeysToUse)

		allPksSum := CombinePubkeys(allPubkeys)

		// Verify the combined signature and public keys.
		ok, err := schnorrVerify(combinedSignature.Serialize(),
			allPksSum, msg, chainhash.HashB)
		if err != nil {
			t.Fatalf("unexpected error %s, ", err)
		}

		if !ok {
			t.Fatalf("expected %v, got %v", true, ok)
		}

		// Corrupt some memory and make sure it breaks something.
		corruptWhat := tRand.Intn(3)
		randItem := tRand.Intn(numKeysForTest - 1)

		// Corrupt private key.
		if corruptWhat == 0 {
			privSerCorrupt := keysToUse[randItem].Serialize()
			pos := tRand.Intn(31)
			bitPos := tRand.Intn(7)
			privSerCorrupt[pos] ^= 1 << uint8(bitPos)
			keysToUse[randItem].D.SetBytes(privSerCorrupt)
		}
		// Corrupt public key.
		if corruptWhat == 1 {
			pubXCorrupt := BigIntToEncodedBytes(pubKeysToUse[randItem].GetX())
			pos := tRand.Intn(31)
			bitPos := tRand.Intn(7)
			pubXCorrupt[pos] ^= 1 << uint8(bitPos)
			pubKeysToUse[randItem].GetX().SetBytes(pubXCorrupt[:])
		}
		// Corrupt private nonce.
		if corruptWhat == 2 {
			privSerCorrupt := privNoncesToUse[randItem].Serialize()
			pos := tRand.Intn(31)
			bitPos := tRand.Intn(7)
			privSerCorrupt[pos] ^= 1 << uint8(bitPos)
			privNoncesToUse[randItem].D.SetBytes(privSerCorrupt)
		}
		// Corrupt public nonce.
		if corruptWhat == 3 {
			pubXCorrupt := BigIntToEncodedBytes(pubNoncesToUse[randItem].GetX())
			pos := tRand.Intn(31)
			bitPos := tRand.Intn(7)
			pubXCorrupt[pos] ^= 1 << uint8(bitPos)
			pubNoncesToUse[randItem].GetX().SetBytes(pubXCorrupt[:])
		}

		for j := range keysToUse {
			thisPubNonce := pubNoncesToUse[j]
			localPubNonces := make([]*secp256k1.PublicKey, numKeysForTest-1)
			itr := 0
			for _, pubNonce := range pubNoncesToUse {
				if bytes.Equal(thisPubNonce.Serialize(), pubNonce.Serialize()) {
					continue
				}
				localPubNonces[itr] = pubNonce
				itr++
			}
			publicNonceSum := CombinePubkeys(localPubNonces)

			sig, _ := schnorrPartialSign(curve, msg, keysToUse[j].Serialize(),
				privNoncesToUse[j].Serialize(), publicNonceSum,
				chainhash.HashB)

			partialSignatures[j] = sig
		}

		// Combine signatures.
		combinedSignature, _ = CombineSigs(curve, partialSignatures)

		// Combine pubkeys.
		allPubkeys = make([]*secp256k1.PublicKey, numKeysForTest)
		copy(allPubkeys, pubKeysToUse)

		allPksSum = CombinePubkeys(allPubkeys)

		// Nothing that makes it here should be valid.
		if allPksSum != nil && combinedSignature != nil {
			ok, _ = schnorrVerify(combinedSignature.Serialize(),
				allPksSum, msg, chainhash.HashB)
			if ok {
				t.Fatalf("expected %v, got %v", false, ok)
			}
		}
	}
}
