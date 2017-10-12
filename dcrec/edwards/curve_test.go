// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package edwards

import (
	"bytes"
	"encoding/hex"
	"math/rand"
	"testing"
)

type XRecoveryVector struct {
	bIn *[32]byte
}

func testPointXRecoveryVectors() []XRecoveryVector {
	r := rand.New(rand.NewSource(54321))

	numCvs := 1000
	cvs := make([]XRecoveryVector, numCvs)
	for i := 0; i < numCvs; i++ {
		bIn := new([32]byte)
		for j := 0; j < fieldIntSize; j++ {
			randByte := r.Intn(255)
			bIn[j] = uint8(randByte)
		}

		cvs[i] = XRecoveryVector{bIn}
		r.Seed(int64(i) + 54321)
	}

	return cvs
}

// Tested functions:
//   BigIntPointToEncodedBytes
//   extendedToBigAffine
//   EncodedBytesToBigIntPoint
func TestXRecovery(t *testing.T) {
	curve := new(TwistedEdwardsCurve)
	curve.InitParam25519()

	for _, vector := range testPointXRecoveryVectors() {
		isNegative := vector.bIn[31]>>7 == 1
		notOnCurve := false
		_, y, err := curve.EncodedBytesToBigIntPoint(vector.bIn)
		// The random point wasn't on the curve.
		if err != nil {
			notOnCurve = true
		}

		if notOnCurve {
			y = EncodedBytesToBigInt(vector.bIn)
		}

		x2 := curve.RecoverXBigInt(isNegative, y)
		if !curve.IsOnCurve(x2, y) {
			if !notOnCurve {
				t.Fatalf("expected %v, got %v", true, notOnCurve)
			}
		} else {
			if notOnCurve {
				t.Fatalf("expected %v, got %v", false, notOnCurve)
			}
			b2 := BigIntPointToEncodedBytes(x2, y)
			if !bytes.Equal(vector.bIn[:], b2[:]) {
				t.Fatalf("expected %x, got %x", vector.bIn[:], b2)
			}
		}

		yFE := EncodedBytesToFieldElement(vector.bIn)
		x3 := curve.RecoverXFieldElement(isNegative, yFE)
		x3BI := FieldElementToBigInt(x3)
		if !curve.IsOnCurve(x3BI, y) {
			if !notOnCurve {
				t.Fatalf("expected %v, got %v", true, notOnCurve)
			}
		} else {
			if notOnCurve {
				t.Fatalf("expected %v, got %v", false, notOnCurve)
			}

			b3 := BigIntPointToEncodedBytes(x3BI, y)
			if !bytes.Equal(vector.bIn[:], b3[:]) {
				t.Fatalf("expected %x, got %x", vector.bIn[:],
					b3)
			}
		}
	}
}

// Tested functions:
//   BigIntPointToEncodedBytes
//   extendedToBigAffine
//   EncodedBytesToBigIntPoint
func TestAdd(t *testing.T) {
	pointHexStrIdx := 0
	pointHexStrSet := []string{
		"4a3f2684abc42977fe50adbb158a9939cc31b210a7c6e6ea4856395ef3e51bf4",
		"eb4c9d80865dc40107846fdbc1e8b3ce3647615877a77b88720d2913adf1f0a4",
		"3e4e5f8276a802b3c8a4bf442a418cc3a435a71ed0f38aa784f9e460d1c57e2e",
		"647f67801b99bf7eb0c00efbc9b63f4246eba59ff21616e85ecf6139e1006f86",
		"213b22c341bc07cac961a066e137f1e43e671a497eba7add362e2abb15475de9",
		"f52f2b3860d5a0a86db4b786dc73d1f3fe29cbfea4b1b0600b58b072d6d25722",
		"4b6b53496e0d93222fb612f02f688914cb93dea6414d510c92a75812976082ed",
		"768fbf82d32f02e26fb7e51bce61abbb6081085026d049fdf42efc6fdc0715be",
		"b66308e6f1080b6f623d8ce4c2537ff3e60d6c5288ae00fdcc9e652ff770d193",
		"e5bbf34b1d308d393b8fee9e3bba1fd66726ceeba2d8be80d46fd74a4eb9187f",
		"a172dd13f4eaf3bd0e3501f3c2edca2ceddbbe05e6a5d503b114d6e9e14522e5",
		"413df123e96ffc6e8d033037cdbe40d70fb9ec17adf547d9f95a2c6e778bb3cb",
		"b3c815edc658038e31fef3e08190bfdfc63640df5e3b490fb50421cc0380bc21",
	}
	curve := new(TwistedEdwardsCurve)
	curve.InitParam25519()
	tpcv := testPointConversionVectors()

	for i := range tpcv {
		if i == 0 {
			continue
		}

		x1, y1, err := curve.EncodedBytesToBigIntPoint(tpcv[i-1].bIn)
		// The random point wasn't on the curve.
		if err != nil {
			continue
		}

		x2, y2, err := curve.EncodedBytesToBigIntPoint(tpcv[i].bIn)
		// The random point wasn't on the curve.
		if err != nil {
			continue
		}

		x, y := curve.Add(x1, y1, x2, y2)
		pointEnc := BigIntPointToEncodedBytes(x, y)
		pointEncAsStr := hex.EncodeToString(pointEnc[:])
		// Assert our results.
		pointHexStr := pointHexStrSet[pointHexStrIdx]
		cmp := pointEncAsStr == pointHexStr
		if !cmp {
			t.Fatalf("expected %v, got %v", true, cmp)
		}
		pointHexStrIdx++
	}
}

type ScalarMultVectorHex struct {
	bIn  string // Point
	s    string // 32 byte scalar
	bRes string // Resulting point
}

func testVectorsScalarMultHex() []ScalarMultVectorHex {
	return []ScalarMultVectorHex{
		{"a4185ac0436e4487cf2c4db66465b4167b0c884be5679ac2c6c5a675c2313216", "3628cff35868bbc95ae4c8d8d0536851a6597e6c96874a2c5b6cee489c1dda56", "0b398bc7c29f05c7d67411824173a9830936273eb4ffd7e546c9ef62bf59f821"},
		{"7b4fae97c829420c9e132f2e1b0ad835f39af9c9d245c87121db68320f957729", "43d12a94e0dfed6489162197ff51769ece9c95a4a784b39926bc5e56703c1554", "d10b9f81cdc426daa6425f9c37057e7090102848927fdde0bb0b07191b33be02"},
		{"5563cfdba3e2653b2b9f8bb43566e0b6b788713196ea65fc6fba9cd760c71205", "ed82427d43dded53c4b4fd204dba3d4bf09cf95e7821cd23c35b36180a23c00e", "f61f95ef730e1183dcdb455faea04a00f81d5a5f15e3ac709a39e0b206c72245"},
		{"b20df028f5ae0409b7131e3165ce5e2c1982e32b6ae8cf2dc9369685444225f3", "19a4cf4eb98082b44460cc0d94ea482c20c96650f71e13d37c9b5b8b132709f9", "0ae96cfa06dd871e57c0423b390e05e3f92071f1efcaa11ba4d8a8a793fd6594"},
		{"42ef8d3ff3b53d14a28992a47201072b4f34d65cc801a033b495502058f136be", "816295ac93196dcd99fd93497c525ae2c77ef28581f287784c18cdedf713ad1c", "df01249832f7802ecc1136946d7d9d422c6db8049f1ec22fe6cdea4732e8c11e"},
		{"58e49f87bc33d252824c2673b1e6d03948a564b095ef0fcc2db9ab068859e761", "4e3604d93eb17aeacb600b5dd101756711e56edeb61b4e096d9831f1b678fecb", "4ea973a59bdab17664ee3e2d2c58048bfe5cb648b8316c170677692317f111d4"},
		{"5b4406c40c5c8edbc4bc560a37336572425348a6613765e7285431855b35c1ae", "98aa0700f27292de4b01a519b322cfa1b6f2d5405c8a0aec011206db44f8661f", "d61f265b145f0a6b6fd03d6cc3fbf533ff3c5b1ab3f6889f357ab1b89fe59ad6"},
		{"b11f09eac50b0e09980f5cbf12511329a7be3c011c7714555ecc99d3058c0c31", "c6b75a8ff9b5d642e78e75f1760de923378e2c117ff55d08e8c9471a9857b1a9", "e9a913f01dcf22c5529041accdcf84991241dd2459450c2cf09b1f7557969cfd"},
		{"2ef244e1a0d29c85a4d9efcc76e73229474221b5dbc7d0a5e5d0a3fe7af2260a", "a17a0742c64b377c4d21b9374c18d2357705129891e493ae0a5ed3944859358d", "4394da4b755f93b5a071084b8d3ddd2d2b6fe051850ce020b78626b2cde3af0f"},
		{"58a70db75e2728e9b8104cb0c0d3a34c0d9f7df962bd49d972c8ffd2f6d166ed", "f257355233d825370122f0b96a2d65862674773e4d8fc8a3e098d91df2269377", "43d399cbc1d906b2070e5bd2ea22ee7ec16acacf1c066eff83b66a4a4e6e045a"},
		{"d28cec42d45b3a3412fa304cf7ec8569799c206c15d660bd397bae71038e2247", "5998863d2c6ec41609ca6ccf759b079c08ff4182080cd43ed94fd50ba76f1250", "c1f277c9a1a0fa5cdac73ddbeaea8b23bf953b8bb116a28927251ca1c66dbcfd"},
		{"4ed7441db6ab3e5eaab93073c324f623c4dd43236b85544243c2ec8f4a470292", "c35e95a67108d5d540f5976ccdfe5efc365daac97678c1754d2de9b64562a5df", "bbb7a5f331f0edec2cbbbf03bc368b9e79be88a40b2ef5d037ff4e5f86526dc2"},
		{"d7ca509d7fd8576dd3e1254f267d25693d610e1641df4aec416ade68eaab750a", "5166fa5902ec25c6956373d1c160a5a187fd265ed1def088b8737b82962c00b9", "1a631f41db3a8aa1e6373b6c1826aed1cf1c5b726dfa3a6f3aedf725139edeb7"},
		{"fb6ef51f9d2d8113e5715f7f383a438e0af9e2a938981bbbaccaf23af1c44846", "bcf743bc9ca831ef5affe4408416526d7f97be16c6489e8480e21cd5a4896230", "6449a7997a35c140e296dd52ca6c2374a39a37a6328b42f80cebcad080ae12fd"},
		{"1c6309c2895303ecb3315b3d557e2544d2d2af78bdbaeda61c9334cedf9416a3", "cf6b3b461f20b49f863292e990b2813b4ea1ca07b1372e423511e01f2ce6aa9f", "9f91711ddf5cc8127bebf3faf1bc919f8db37ab35c4a8ab621314d4532568ab7"},
		{"1433d904b51a4a9d3654b8415fde9ce577652e64db634d0d8a103bfbd5f4d259", "56ec65e69dd0f10377c764fa5caca260af73d44a9ed2b838a3485055cd6216e1", "ef18bc7bea44254276a8755940ee97fa436b4ac904f9e8232af69591dcca99ad"},
		{"662bdf3aabd9fc2c252ce38a272ac84303211653e0d88bc9b25ae4a6b9009813", "1472fd6a26b8ac15518eb66c4fab44f28568f3f127d40d70b643fc1b85ec35cc", "ca2627334a90483fa6f2532e83fc38fed0e75f2388feee99cc69102297508b23"},
		{"a2e6a7c28f87bb95ea611fcfc3e804dd772e3b6dad6e60b3b6d3aed1136bc009", "cd9a9ffe3a346baa5ac98c504d197690139b53ebb01227dcebda6de94143c9ee", "fc010f6110e82a1fc9df7c3d0e9b3d6bd550e32d41bb4aaca4330086faa974df"},
		{"0d2ed068749d471e0ccc53cbdcbb11022f2f6e5fe82faaf662566739ad93a46a", "5102094e6e5ede9a1d74dad9f41f19842f32a378e92f9b8fe46c49513148ba98", "adcfbb2da1b6b3fdf49ff91a103ce18b5fa7cdc03dc96d5eea62db88fef24444"},
		{"3025fb1259e8bf5b629d167035a83bc4bae1a4d92790706da471c77e83bbff65", "46ddd06a554a5d65a0bb513ccfa962d27e8bb658f2c0c08124ea8b61b3912457", "afa21072bffee77e9df0d0d3afae7c31723cc7a52f82269a315f385b5b19b9d4"},
		{"b3429d95ac09581860251b1da2d54c6d361e3e3a116d0f153c9d29a8b2eb14e9", "854ca13b76c837eda31fb06232c8e2c56396a09a8c4fcc517a5a54c1dc2d01ea", "298dccfe0d70ce69e0e12613008901b484c7c6b9ae3c62cbe5152b8a12c22296"},
		{"efa0976786194dd52174f436ea27d4e7dc9599f7ce4424bfbac4cdc8f05388d8", "1f253bc2a828874fc46adc255f6203928bb77a84b7d545ee3746c4686d411206", "71851029252000821bd156cce117ab193471f1e2eba21897403bf8307bc666e4"},
		{"fbc1f969efdd138c7c3cfeb7cfa906ca20c3522533a14745350074a57045f411", "3f82d419f4da810ca0e64a4e6e5fb3011b919417d2edc3e01a4cdb8fb97ed8a5", "15f9defa2f0b6ceca4b298535a924a55f8b45ce9ee101f9b273744dab6783704"},
		{"9777c96b78a9878add14578e759246aeb7678554ea843439acd346d24563ee52", "c67fd95f24cf5b7dab1bd3fe84d6ab0b5a751ee6f125d9693f96da90b57ae366", "9cef760dd9d3371f68948f6c3b8b60b3a3e5e4d2d6f7f2204e391786f66f53ab"},
		{"58848e020b5d60fafbfb1b193b6d011ddc4342b4fd9846ff53793d780ffd97a9", "35ce524d1411196917a8819bd5e14482867b0443b675d08bddf7a363d03d8743", "fde89cbd9c19e4d6e6bcf1a8aa8243954249cf19d2d0910b38bffb5b7cf40289"},
		{"3e4ba1e84c427835ec9d7f5f1ad242d286f691ee7193c0ed4b9cf17ce38388e3", "4292f0dbedcffeecb0e26c4d56debc2a9b22e963bd430a44942a6538677137e9", "56b47522d793c0f3fed33bec100fa0dbdec79f5e00ca9e7003884410e8b7f8a1"},
		{"f330b3cdeef0b31f1e8e0787cee38e81216a0a90f536c81c33293d577ec32cb4", "143aff9e186d585b9167eb60780f2ed424751431d141e19ae42d6745cbd77e41", "d82cb0b18e90f2bdf2fc6b0ac34ec53325e19262dec88c22eddc4874207c35b6"},
		{"adf0a708f1c3e0a0e56b1d74f91861462a2c20619afded35560e6a57db9df026", "de62849260fae4e4f4a707ffa0f95a6a7b1a16b2b6083fa93f4f7bfbaeb6823f", "7362db606fafb2c5b336eb36760ea87cb7412fe1e2ac152f22450074e6a14972"},
		{"48c1eb698fc29e1ffb28d5bbdddd896343935d574e4800c9ec336da133db7253", "41a37cae219b9b5097ecb76530f3de685d7c7aaa48f8a5237a6525bb84d4c9b7", "d834ea9ea06272263059948e107c8f083c197a76a3dec6b76ee169729ee6f11f"},
		{"93cc6d1727b2bd5f918563cf8571a9c0f6b771c25cb9aa47ebdff08bbbf00f17", "ad00a73d9a3f54b29e3e3a65d4b35f28aa0bc6b3aa1e82d628489368aa4db441", "6f4789dd5250e097c2c2025f31920d974dc58ce5d204437a2db578ae5d2aa544"},
		{"4f651b5f0918f14ea45433ed332dc4bf46456e076903ede2313694b96faa8c40", "12a331c658d41a8a97de1db8bd392b53558fb2f1724c904d476652d5e82eea95", "0fa278fb83a017f1058ef7fb937747ab8646d4d5cac694a4a3e1b24340ba71d9"},
	}
}

type ScalarMultVector struct {
	bIn  *[32]byte // Point
	s    *[32]byte // 32 byte scalar
	bRes *[32]byte // Resulting point
}

func testVectorsScalarMult() []ScalarMultVector {
	tvsmh := testVectorsScalarMultHex()
	var tvsms []ScalarMultVector
	for _, v := range tvsmh {
		bIn, _ := hex.DecodeString(v.bIn)
		s, _ := hex.DecodeString(v.s)
		bRes, _ := hex.DecodeString(v.bRes)
		lv := ScalarMultVector{copyBytes(bIn), copyBytes(s), copyBytes(bRes)}
		tvsms = append(tvsms, lv)
	}

	return tvsms
}

// Tested functions:
//   Add
//   Double
//   ScalarMult
func TestScalarMult(t *testing.T) {
	curve := new(TwistedEdwardsCurve)
	curve.InitParam25519()

	for _, vector := range testVectorsScalarMult() {
		x, y, _ := curve.EncodedBytesToBigIntPoint(vector.bIn)
		sBig := EncodedBytesToBigInt(vector.s) // We need big endian
		xMul, yMul := curve.ScalarMult(x, y, sBig.Bytes())
		finalPoint := BigIntPointToEncodedBytes(xMul, yMul)
		cmp := bytes.Equal(vector.bRes[:], finalPoint[:])
		if !cmp {
			t.Fatalf("expected %v, got %v", true, cmp)
		}
	}
}
