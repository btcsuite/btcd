package v2transport

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ellswift"
)

func setHex(hexString string) *btcec.FieldVal {
	if len(hexString)%2 != 0 {
		hexString = "0" + hexString
	}
	bytes, _ := hex.DecodeString(hexString)

	var f btcec.FieldVal
	f.SetByteSlice(bytes)

	return &f
}

const (
	mainNet = 0xd9b4bef9
)

func TestPacketEncodingVectors(t *testing.T) {
	tests := []struct {
		inIdx                 int
		inPrivOurs            string
		inEllswiftOurs        string
		inEllswiftTheirs      string
		inInitiating          bool
		inContents            string
		inMultiply            int
		inAad                 string
		inIgnore              bool
		midXOurs              string
		midXTheirs            string
		midXShared            string
		midSharedSecret       string
		midInitiatorL         string
		midInitiatorP         string
		midResponderL         string
		midResponderP         string
		midSendGarbageTerm    string
		midRecvGarbageTerm    string
		outSessionID          string
		outCiphertext         string
		outCiphertextEndsWith string
	}{
		{
			inIdx:                 1,
			inPrivOurs:            "61062ea5071d800bbfd59e2e8b53d47d194b095ae5a4df04936b49772ef0d4d7",
			inEllswiftOurs:        "ec0adff257bbfe500c188c80b4fdd640f6b45a482bbc15fc7cef5931deff0aa186f6eb9bba7b85dc4dcc28b28722de1e3d9108b985e2967045668f66098e475b",
			inEllswiftTheirs:      "a4a94dfce69b4a2a0a099313d10f9f7e7d649d60501c9e1d274c300e0d89aafaffffffffffffffffffffffffffffffffffffffffffffffffffffffff8faf88d5",
			inInitiating:          true,
			inContents:            "8e",
			inMultiply:            1,
			inAad:                 "",
			inIgnore:              false,
			midXOurs:              "19e965bc20fc40614e33f2f82d4eeff81b5e7516b12a5c6c0d6053527eba0923",
			midXTheirs:            "0c71defa3fafd74cb835102acd81490963f6b72d889495e06561375bd65f6ffc",
			midXShared:            "4eb2bf85bd00939468ea2abb25b63bc642e3d1eb8b967fb90caa2d89e716050e",
			midSharedSecret:       "c6992a117f5edbea70c3f511d32d26b9798be4b81a62eaee1a5acaa8459a3592",
			midInitiatorL:         "9a6478b5fbab1f4dd2f78994b774c03211c78312786e602da75a0d1767fb55cf",
			midInitiatorP:         "7d0c7820ba6a4d29ce40baf2caa6035e04f1e1cefd59f3e7e59e9e5af84f1f51",
			midResponderL:         "17bc726421e4054ac6a1d54915085aaa766f4d3cf67bbd168e6080eac289d15e",
			midResponderP:         "9f0fc1c0e85fd9a8eee07e6fc41dba2ff54c7729068a239ac97c37c524cca1c0",
			midSendGarbageTerm:    "faef555dfcdb936425d84aba524758f3",
			midRecvGarbageTerm:    "02cb8ff24307a6e27de3b4e7ea3fa65b",
			outSessionID:          "ce72dffb015da62b0d0f5474cab8bc72605225b0cee3f62312ec680ec5f41ba5",
			outCiphertext:         "7530d2a18720162ac09c25329a60d75adf36eda3c3",
			outCiphertextEndsWith: "",
		},
		{
			inIdx:                 999,
			inPrivOurs:            "1f9c581b35231838f0f17cf0c979835baccb7f3abbbb96ffcc318ab71e6e126f",
			inEllswiftOurs:        "a1855e10e94e00baa23041d916e259f7044e491da6171269694763f018c7e63693d29575dcb464ac816baa1be353ba12e3876cba7628bd0bd8e755e721eb0140",
			inEllswiftTheirs:      "fffffffffffffffffffffffffffffffffffffffffffffffffffffffefffffc2f0000000000000000000000000000000000000000000000000000000000000000",
			inInitiating:          false,
			inContents:            "3eb1d4e98035cfd8eeb29bac969ed3824a",
			inMultiply:            1,
			inAad:                 "",
			inIgnore:              false,
			midXOurs:              "45b6f1f684fd9f2b16e2651ddc47156c0695c8c5cd2c0c9df6d79a1056c61120",
			midXTheirs:            "edd1fd3e327ce90cc7a3542614289aee9682003e9cf7dcc9cf2ca9743be5aa0c",
			midXShared:            "c40eb6190caf399c9007254ad5e5fa20d64af2b41696599c59b2191d16992955",
			midSharedSecret:       "a0138f564f74d0ad70bc337dacc9d0bf1d2349364caf1188a1e6e8ddb3b7b184",
			midInitiatorL:         "b82a0a7ce7219777f914d2ab873c5c487c56bd7b68622594d67fe029a8fa7def",
			midInitiatorP:         "d760ba8f62dd3d29d7d5584e310caf2540285edc6b51c640f9497e99c3536fd2",
			midResponderL:         "9db0c6f9a903cbab5d7b3c58273a3421eec0001814ec53236bd405131a0d8e90",
			midResponderP:         "23d2b5e653e6a3a8db160a2ca03d11cb5a79983babba861fcb57c38413323c0c",
			midSendGarbageTerm:    "efb64fd80acd3825ac9bc2a67216535a",
			midRecvGarbageTerm:    "b3cb553453bceb002897e751ff7588bf",
			outSessionID:          "9267c54560607de73f18c563b76a2442718879c52dd39852885d4a3c9912c9ea",
			outCiphertext:         "1da1bcf589f9b61872f45b7fa5371dd3f8bdf5d515b0c5f9fe9f0044afb8dc0aa1cd39a8c4",
			outCiphertextEndsWith: "",
		},
		{
			inIdx:                 0,
			inPrivOurs:            "0286c41cd30913db0fdff7a64ebda5c8e3e7cef10f2aebc00a7650443cf4c60d",
			inEllswiftOurs:        "d1ee8a93a01130cbf299249a258f94feb5f469e7d0f2f28f69ee5e9aa8f9b54a60f2c3ff2d023634ec7f4127a96cc11662e402894cf1f694fb9a7eaa5f1d9244",
			inEllswiftTheirs:      "ffffffffffffffffffffffffffffffffffffffffffffffffffffffff22d5e441524d571a52b3def126189d3f416890a99d4da6ede2b0cde1760ce2c3f98457ae",
			inInitiating:          true,
			inContents:            "054290a6c6ba8d80478172e89d32bf690913ae9835de6dcf206ff1f4d652286fe0ddf74deba41d55de3edc77c42a32af79bbea2c00bae7492264c60866ae5a",
			inMultiply:            1,
			inAad:                 "84932a55aac22b51e7b128d31d9f0550da28e6a3f394224707d878603386b2f9d0c6bcd8046679bfed7b68c517e7431e75d9dd34605727d2ef1c2babbf680ecc8d68d2c4886e9953a4034abde6da4189cd47c6bb3192242cf714d502ca6103ee84e08bc2ca4fd370d5ad4e7d06c7fbf496c6c7cc7eb19c40c61fb33df2a9ba48497a96c98d7b10c1f91098a6b7b16b4bab9687f27585ade1491ae0dba6a79e1e2d85dd9d9d45c5135ca5fca3f0f99a60ea39edbc9efc7923111c937913f225d67788d5f7e8852b697e26b92ec7bfcaa334a1665511c2b4c0a42d06f7ab98a9719516c8fd17f73804555ee84ab3b7d1762f6096b778d3cb9c799cbd49a9e4a325197b4e6cc4a5c4651f8b41ff88a92ec428354531f970263b467c77ed11312e2617d0d53fe9a8707f51f9f57a77bfb49afe3d89d85ec05ee17b9186f360c94ab8bb2926b65ca99dae1d6ee1af96cad09de70b6767e949023e4b380e66669914a741ed0fa420a48dbc7bfae5ef2019af36d1022283dd90655f25eec7151d471265d22a6d3f91dc700ba749bb67c0fe4bc0888593fbaf59d3c6fff1bf756a125910a63b9682b597c20f560ecb99c11a92c8c8c3f7fbfaa103146083a0ccaecf7a5f5e735a784a8820155914a289d57d8141870ffcaf588882332e0bcd8779efa931aa108dab6c3cce76691e345df4a91a03b71074d66333fd3591bff071ea099360f787bbe43b7b3dff2a59c41c7642eb79870222ad1c6f2e5a191ed5acea51134679587c9cf71c7d8ee290be6bf465c4ee47897a125708704ad610d8d00252d01959209d7cd04d5ecbbb1419a7e84037a55fefa13dee464b48a35c96bcb9a53e7ed461c3a1607ee00c3c302fd47cd73fda7493e947c9834a92d63dcfbd65aa7c38c3e3a2748bb5d9a58e7495d243d6b741078c8f7ee9c8813e473a323375702702b0afae1550c8341eedf5247627343a95240cb02e3e17d5dca16f8d8d3b2228e19c06399f8ec5c5e9dbe4caef6a0ea3ffb1d3c7eac03ae030e791fa12e537c80d56b55b764cadf27a8701052df1282ba8b5e3eb62b5dc7973ac40160e00722fa958d95102fc25c549d8c0e84bed95b7acb61ba65700c4de4feebf78d13b9682c52e937d23026fb4c6193e6644e2d3c99f91f4f39a8b9fc6d013f89c3793ef703987954dc0412b550652c01d922f525704d32d70d6d4079bc3551b563fb29577b3aecdc9505011701dddfd94830431e7a4918927ee44fb3831ce8c4513839e2deea1287f3fa1ab9b61a256c09637dbc7b4f0f8fbb783840f9c24526da883b0df0c473cf231656bd7bc1aaba7f321fec0971c8c2c3444bff2f55e1df7fea66ec3e440a612db9aa87bb505163a59e06b96d46f50d8120b92814ac5ab146bc78dbbf91065af26107815678ce6e33812e6bf3285d4ef3b7b04b076f21e7820dcbfdb4ad5218cf4ff6a65812d8fcb98ecc1e95e2fa58e3efe4ce26cd0bd400d6036ab2ad4f6c713082b5e3f1e04eb9e3b6c8f63f57953894b9e220e0130308e1fd91f72d398c1e7962ca2c31be83f31d6157633581a0a6910496de8d55d3d07090b6aa087159e388b7e7dec60f5d8a60d93ca2ae91296bd484d916bfaaa17c8f45ea4b1a91b37c82821199a2b7596672c37156d8701e7352aa48671d3b1bbbd2bd5f0a2268894a25b0cb2514af39c8743f8cce8ab4b523053739fd8a522222a09acf51ac704489cf17e4b7125455cb8f125b4d31af1eba1f8cf7f81a5a100a141a7ee72e8083e065616649c241f233645c5fc865d17f0285f5c52d9f45312c979bfb3ce5f2a1b951deddf280ffb3f370410cffd1583bfa90077835aa201a0712d1dcd1293ee177738b14e6b5e2a496d05220c3253bb6578d6aff774be91946a614dd7e879fb3dcf7451e0b9adb6a8c44f53c2c464bcc0019e9fad89cac7791a0a3f2974f759a9856351d4d2d7c5612c17cfc50f8479945df57716767b120a590f4bf656f4645029a525694d8a238446c5f5c2c1c995c09c1405b8b1eb9e0352ffdf766cc964f8dcf9f8f043dfab6d102cf4b298021abd78f1d9025fa1f8e1d710b38d9d1652f2d88d1305874ec41609b6617b65c5adb19b6295dc5c5da5fdf69f28144ea12f17c3c6fcce6b9b5157b3dfc969d6725fa5b098a4d9b1d31547ed4c9187452d281d0a5d456008caf1aa251fac8f950ca561982dc2dc908d3691ee3b6ad3ae3d22d002577264ca8e49c523bd51c4846be0d198ad9407bf6f7b82c79893eb2c05fe9981f687a97a4f01fe45ff8c8b7ecc551135cd960a0d6001ad35020be07ffb53cb9e731522ca8ae9364628914b9b8e8cc2f37f03393263603cc2b45295767eb0aac29b0930390eb89587ab2779d2e3decb8042acece725ba42eda650863f418f8d0d50d104e44fbbe5aa7389a4a144a8cecf00f45fb14c39112f9bfb56c0acbd44fa3ff261f5ce4acaa5134c2c1d0cca447040820c81ab1bcdc16aa075b7c68b10d06bbb7ce08b5b805e0238f24402cf24a4b4e00701935a0c68add3de090903f9b85b153cb179a582f57113bfc21c2093803f0cfa4d9d4672c2b05a24f7e4c34a8e9101b70303a7378b9c50b6cddd46814ef7fd73ef6923feceab8fc5aa8b0d185f2e83c7a99dcb1077c0ab5c1f5d5f01ba2f0420443f75c4417db9ebf1665efbb33dca224989920a64b44dc26f682cc77b4632c8454d49135e52503da855bc0f6ff8edc1145451a9772c06891f41064036b66c3119a0fc6e80dffeb65dc456108b7ca0296f4175fff3ed2b0f842cd46bd7e86f4c62dfaf1ddbf836263c00b34803de164983d0811cebfac86e7720c726d3048934c36c23189b02386a722ca9f0fe00233ab50db928d3bccea355cc681144b8b7edcaae4884d5a8f04425c0890ae2c74326e138066d8c05f4c82b29df99b034ea727afde590a1f2177ace3af99cfb1729d6539ce7f7f7314b046aab74497e63dd399e1f7d5f16517c23bd830d1fdee810f3c3b77573dd69c4b97d80d71fb5a632e00acdfa4f8e829faf3580d6a72c40b28a82172f8dcd4627663ebf6069736f21735fd84a226f427cd06bb055f94e7c92f31c48075a2955d82a5b9d2d0198ce0d4e131a112570a8ee40fb80462a81436a58e7db4e34b6e2c422e82f934ecda9949893da5730fc5c23c7c920f363f85ab28cc6a4206713c3152669b47efa8238fa826735f17b4e78750276162024ec85458cd5808e06f40dd9fd43775a456a3ff6cae90550d76d8b2899e0762ad9a371482b3e38083b1274708301d6346c22fea9bb4b73db490ff3ab05b2f7f9e187adef139a7794454b7300b8cc64d3ad76c0e4bc54e08833a4419251550655380d675bc91855aeb82585220bb97f03e976579c08f321b5f8f70988d3061f41465517d53ac571dbf1b24b94443d2e9a8e8a79b392b3d6a4ecdd7f626925c365ef6221305105ce9b5f5b6ecc5bed3d702bd4b7f5008aa8eb8c7aa3ade8ecf6251516fbefeea4e1082aa0e1848eddb31ffe44b04792d296054402826e4bd054e671f223e5557e4c94f89ca01c25c44f1a2ff2c05a70b43408250705e1b858bf0670679fdcd379203e36be3500dd981b1a6422c3cf15224f7fefdef0a5f225c5a09d15767598ecd9e262460bb33a4b5d09a64591efabc57c923d3be406979032ae0bc0997b65336a06dd75b253332ad6a8b63ef043f780a1b3fb6d0b6cad98b1ef4a02535eb39e14a866cfc5fc3a9c5deb2261300d71280ebe66a0776a151469551c3c5fa308757f956655278ec6330ae9e3625468c5f87e02cd9a6489910d4143c1f4ee13aa21a6859d907b788e28572fecee273d44e4a900fa0aa668dd861a60fb6b6b12c2c5ef3c8df1bd7ef5d4b0d1cdb8c15fffbb365b9784bd94abd001c6966216b9b67554ad7cb7f958b70092514f7800fc40244003e0fd1133a9b850fb17f4fcafde07fc87b07fb510670654a5d2d6fc9876ac74728ea41593beef003d6858786a52d3a40af7529596767c17000bfaf8dc52e871359f4ad8bf6e7b2853e5229bdf39657e213580294a5317c5df172865e1e17fe37093b585e04613f5f078f761b2b1752eb32983afda24b523af8851df9a02b37e77f543f18888a782a994a50563334282bf9cdfccc183fdf4fcd75ad86ee0d94f91ee2300a5befbccd14e03a77fc031a8cfe4f01e4c5290f5ac1da0d58ea054bd4837cfd93e5e34fc0eb16e48044ba76131f228d16cde9b0bb978ca7cdcd10653c358bdb26fdb723a530232c32ae0a4cecc06082f46e1c1d596bfe60621ad1e354e01e07b040cc7347c016653f44d926d13ca74e6cbc9d4ab4c99f4491c95c76fff5076b3936eb9d0a286b97c035ca88a3c6309f5febfd4cdaac869e4f58ed409b1e9eb4192fb2f9c2f12176d460fd98286c9d6df84598f260119fd29c63f800c07d8df83d5cc95f8c2fea2812e7890e8a0718bb1e031ecbebc0436dcf3e3b9a58bcc06b4c17f711f80fe1dffc3326a6eb6e00283055c6dabe20d311bfd5019591b7954f8163c9afad9ef8390a38f3582e0a79cdf0353de8eeb6b5f9f27b16ffdef7dd62869b4840ee226ccdce95e02c4545eb981b60571cd83f03dc5eaf8c97a0829a4318a9b3dc06c0e003db700b2260ff1fa8fee66890e637b109abb03ec901b05ca599775f48af50154c0e67d82bf0f558d7d3e0778dc38bea1eb5f74dc8d7f90abdf5511a424be66bf8b6a3cacb477d2e7ef4db68d2eba4d5289122d851f9501ba7e9c4957d8eba3be3fc8e785c4265a1d65c46f2809b70846c693864b169c9dcb78be26ea14b8613f145b01887222979a9e67aee5f800caa6f5c4229bdeefc901232ace6143c9865e4d9c07f51aa200afaf7e48a7d1d8faf366023beab12906ffcb3eaf72c0eb68075e4daf3c080e0c31911befc16f0cc4a09908bb7c1e26abab38bd7b788e1a09c0edf1a35a38d2ff1d3ed47fcdaae2f0934224694f5b56705b9409b6d3d64f3833b686f7576ec64bbdd6ff174e56c2d1edac0011f904681a73face26573fbba4e34652f7ae84acfb2fa5a5b3046f98178cd0831df7477de70e06a4c00e305f31aafc026ef064dd68fd3e4252b1b91d617b26c6d09b6891a00df68f105b5962e7f9d82da101dd595d286da721443b72b2aba2377f6e7772e33b3a5e3753da9c2578c5d1daab80187f55518c72a64ee150a7cb5649823c08c9f62cd7d020b45ec2cba8310db1a7785a46ab24785b4d54ff1660b5ca78e05a9a55edba9c60bf044737bc468101c4e8bd1480d749be5024adefca1d998abe33eaeb6b11fbb39da5d905fdd3f611b2e51517ccee4b8af72c2d948573505590d61a6783ab7278fc43fe55b1fcc0e7216444d3c8039bb8145ef1ce01c50e95a3f3feab0aee883fdb94cc13ee4d21c542aa795e18932228981690f4d4c57ca4db6eb5c092e29d8a05139d509a8aeb48baa1eb97a76e597a32b280b5e9d6c36859064c98ff96ef5126130264fa8d2f49213870d9fb036cff95da51f270311d9976208554e48ffd486470d0ecdb4e619ccbd8226147204baf8e235f54d8b1cba8fa34a9a4d055de515cdf180d2bb6739a175183c472e30b5c914d09eeb1b7dafd6872b38b48c6afc146101200e6e6a44fe5684e220adc11f5c403ddb15df8051e6bdef09117a3a5349938513776286473a3cf1d2788bb875052a2e6459fa7926da33380149c7f98d7700528a60c954e6f5ecb65842fde69d614be69eaa2040a4819ae6e756accf936e14c1e894489744a79c1f2c1eb295d13e2d767c09964b61f9cfe497649f712",
			inIgnore:              false,
			midXOurs:              "33a32d10066fa3963a9518a14d1bd1cb5ccaceaeaaeddb4d7aead90c08395bfd",
			midXTheirs:            "568146140669e69646a6ffeb3793e8010e2732209b4c34ec13e209a070109183",
			midXShared:            "a1017beaa8784f283dee185cd847ae3a327a981e62ae21e8c5face175fc97e9b",
			midSharedSecret:       "250b93570d411149105ab8cb0bc5079914906306368c23e9d77c2a33265b994c",
			midInitiatorL:         "4ec7daf7294a4a2c717442dd21cf2f052a3bfe9d535b55da0f66fecf87a27534",
			midInitiatorP:         "52ab4db9c4b06621f8ded3405691eb32465b1360d15a6b127ded4d15f9cde466",
			midResponderL:         "ba9906da802407ddedf6733e29f3996c62425e79d3cbfeebbd6ec4cdc7c976a8",
			midResponderP:         "ee661e18c97319ad071106bf35fe1085034832f70718d92f887932128b6100c7",
			midSendGarbageTerm:    "d4e3f18ac2e2095edb5c3b94236118ad",
			midRecvGarbageTerm:    "4faa6c4233d9fd53d170ede4172142a8",
			outSessionID:          "23f154ac43cfc59c4243e9fc68aeec8f19ad3942d74108e833b36f0dd3dcd357",
			outCiphertext:         "8da7de6ea7bf2a81a396a42880ba1f5756734c4821309ac9aeffa2a26ce86873b9dc4935a772de6ec5162c6d075b14536800fb174841153511bfb597e992e2fe8a450c4bce102cc550bb37fd564c4d60bf884e",
			outCiphertextEndsWith: "",
		},
		{
			inIdx:                 223,
			inPrivOurs:            "6c77432d1fda31e9f942f8af44607e10f3ad38a65f8a4bddae823e5eff90dc38",
			inEllswiftOurs:        "d2685070c1e6376e633e825296634fd461fa9e5bdf2109bcebd735e5a91f3e587c5cb782abb797fbf6bb5074fd1542a474f2a45b673763ec2db7fb99b737bbb9",
			inEllswiftTheirs:      "56bd0c06f10352c3a1a9f4b4c92f6fa2b26df124b57878353c1fc691c51abea77c8817daeeb9fa546b77c8daf79d89b22b0e1b87574ece42371f00237aa9d83a",
			inInitiating:          false,
			inContents:            "7e0e78eb6990b059e6cf0ded66ea93ef82e72aa2f18ac24f2fc6ebab561ae557420729da103f64cecfa20527e15f9fb669a49bbbf274ef0389b3e43c8c44e5f60bf2ac38e2b55e7ec4273dba15ba41d21f8f5b3ee1688b3c29951218caf847a97fb50d75a86515d445699497d968164bf740012679b8962de573be941c62b7ef",
			inMultiply:            1,
			inAad:                 "",
			inIgnore:              true,
			midXOurs:              "193d019db571162e52567e0cfdf9dd6964394f32769ae2edc4933b03b502d771",
			midXTheirs:            "2dd7b9cc85524f8670f695c3143ac26b45cebcabb2782a85e0fe15aee3956535",
			midXShared:            "5e35f94adfd57976833bffec48ef6dde983d18a55501154191ea352ef06732ee",
			midSharedSecret:       "1918b741ef5f9d1d7670b050c152b4a4ead2c31be9aecb0681c0cd4324150853",
			midInitiatorL:         "97124c56236425d792b1ec85e34b846e8d88c9b9f1d4f23ac6cdcc4c177055a0",
			midInitiatorP:         "8c71b468c61119415e3c1dfdd184134211951e2f623199629a46bff9673611f2",
			midResponderL:         "b43b8791b51ed682f56d64351601be28e478264411dcf963b14ee60b9ae427fa",
			midResponderP:         "794dde4b38ef04250c534a7fa638f2e8cc8b6d2c6110ec290ab0171fdf277d51",
			midSendGarbageTerm:    "cf2e25f23501399f30738d7eee652b90",
			midRecvGarbageTerm:    "225a477a28a54ea7671d2b217a9c29db",
			outSessionID:          "7ec02fea8c1484e3d0875f978c5f36d63545e2e4acf56311394422f4b66af612",
			outCiphertext:         "",
			outCiphertextEndsWith: "729847a3e9eba7a5bff454b5de3b393431ee360736b6c030d7a5bd01d1203d2e98f528543fd2bf886ccaa1ada5e215a730a36b3f4abfc4e252c89eb01d9512f94916dae8a76bf16e4da28986ffe159090fe5267ee3394300b7ccf4dfad389a26321b3a3423e4594a82ccfbad16d6561ecb8772b0cb040280ff999a29e3d9d4fd",
		},
		{
			inIdx:                 448,
			inPrivOurs:            "a6ec25127ca1aa4cf16b20084ba1e6516baae4d32422288e9b36d8bddd2de35a",
			inEllswiftOurs:        "ffffffffffffffffffffffffffffffffffffffffffffffffffffffff053d7ecca53e33e185a8b9be4e7699a97c6ff4c795522e5918ab7cd6b6884f67e683f3dc",
			inEllswiftTheirs:      "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffa7730be30000000000000000000000000000000000000000000000000000000000000000",
			inInitiating:          true,
			inContents:            "00cf68f8f7ac49ffaa02c4864fdf6dfe7bbf2c740b88d98c50ebafe32c92f3427f57601ffcb21a3435979287db8fee6c302926741f9d5e464c647eeb9b7acaeda46e00abd7506fc9a719847e9a7328215801e96198dac141a15c7c2f68e0690dd1176292a0dded04d1f548aad88f1aebdc0a8f87da4bb22df32dd7c160c225b843e83f6525d6d484f502f16d923124fc538794e21da2eb689d18d87406ecced5b9f92137239ed1d37bcfa7836641a83cf5e0a1cf63f51b06f158e499a459ede41c",
			inMultiply:            1,
			inAad:                 "",
			inIgnore:              false,
			midXOurs:              "02b225089255f7b02b20276cfe9779144df8fb1957b477bff3239d802d1256e9",
			midXTheirs:            "5232c4b6bde9d3d45d7b763ebd7495399bb825cc21de51011761cd81a51bdc84",
			midXShared:            "379223d2f1ea7f8a22043c4ce4122623098309e15b1ce58286ebe3d3bf40f4e1",
			midSharedSecret:       "dd210aa6629f20bb328e5d89daa6eb2ac3d1c658a725536ff154f31b536c23b2",
			midInitiatorL:         "393472f85a5cc6b0f02c4bd466db7a2dc5b91fc9dcb15c0dd6dc21116ece8bca",
			midInitiatorP:         "c80b87b793db47320b2795db66d331bd3021cc24e360d59d0fa8974f54687e0c",
			midResponderL:         "ef16a43d77e2b270b0a145ee1618d35f3c943cc7877d6cfcff2287d41692be39",
			midResponderP:         "20d4b62e2d982c61bb0cc39a93283d98af36530ef12331d44b2477b0e521b490",
			midSendGarbageTerm:    "fead69be77825a23daec377c362aa560",
			midRecvGarbageTerm:    "511d4980526c5e64aa7187462faeafdd",
			outSessionID:          "acb8f084ea763ddd1b92ac4ed23bf44de20b84ab677d4e4e6666a6090d40353d",
			outCiphertext:         "",
			outCiphertextEndsWith: "77b4656934a82de1a593d8481f020194ddafd8cac441f9d72aeb8721e6a14f49698ca6d9b2b6d59d07a01aa552fd4d5b68d0d1617574c77dea10bfadbaa31b83885b7ceac2fd45e3e4a331c51a74e7b1698d81b64c87c73c5b9258b4d83297f9debc2e9aa07f8572ff434dc792b83ecf07b3197de8dc9cf7be56acb59c66cff5",
		},
		{
			inIdx:                 673,
			inPrivOurs:            "0af952659ed76f80f585966b95ab6e6fd68654672827878684c8b547b1b94f5a",
			inEllswiftOurs:        "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffc81017fd92fd31637c26c906b42092e11cc0d3afae8d9019d2578af22735ce7bc469c72d",
			inEllswiftTheirs:      "9652d78baefc028cd37a6a92625b8b8f85fde1e4c944ad3f20e198bef8c02f19fffffffffffffffffffffffffffffffffffffffffffffffffffffffff2e91870",
			inInitiating:          false,
			inContents:            "5c6272ee55da855bbbf7b1246d9885aa7aa601a715ab86fa46c50da533badf82b97597c968293ae04e",
			inMultiply:            97561,
			inAad:                 "",
			inIgnore:              false,
			midXOurs:              "4b1767466fe2fb8deddf2dc52cc19c7e2032007e19bfb420b30a80152d0f22d6",
			midXTheirs:            "64c383e0e78ac99476ddff2061683eeefa505e3666673a1371342c3e6c26981d",
			midXShared:            "5bcfeac98d87e87e158bf839f1269705429f7af2a25b566a25811b5f9aef9560",
			midSharedSecret:       "3568f2aea2e14ef4ee4a3c2a8b8d31bc5e3187ba86db10739b4ff8ec92ff6655",
			midInitiatorL:         "c7df866a62b7d404eb530b2be245a7aece0fb4791402a1de8f33530cbf777cc1",
			midInitiatorP:         "8f732e4aae2ba9314e0982492fa47954de9c189d92fbc549763b27b1b47642ce",
			midResponderL:         "992085edfecb92c62a3a7f96ea416f853f34d0dfe065b966b6968b8b87a83081",
			midResponderP:         "c5ba5eaf9e1c807154ebab3ea472499e815a7be56dfaf0c201cf6e91ffeca8e6",
			midSendGarbageTerm:    "5e2375ac629b8df1e4ff3617c6255a70",
			midRecvGarbageTerm:    "70bcbffcb62e4d29d2605d30bceef137",
			outSessionID:          "7332e92a3f9d2792c4d444fac5ed888c39a073043a65eefb626318fd649328f8",
			outCiphertext:         "",
			outCiphertextEndsWith: "657a4a19711ce593c3844cb391b224f60124aba7e04266233bc50cafb971e26c7716b76e98376448f7d214dd11e629ef9a974d60e3770a695810a61c4ba66d78b936ee7892b98f0b48ddae9fcd8b599dca1c9b43e9b95e0226cf8d4459b8a7c2c4e6db80f1d58c7b20dd7208fa5c1057fb78734223ee801dbd851db601fee61e",
		},
		{
			inIdx:                 1024,
			inPrivOurs:            "f90e080c64b05824c5a24b2501d5aeaf08af3872ee860aa80bdcd430f7b63494",
			inEllswiftOurs:        "ffffffffffffffffffffffffffffffffffffffffffffffffffffffff115173765dc202cf029ad3f15479735d57697af12b0131dd21430d5772e4ef11474d58b9",
			inEllswiftTheirs:      "12a50f3fafea7c1eeada4cf8d33777704b77361453afc83bda91eef349ae044d20126c6200547ea5a6911776c05dee2a7f1a9ba7dfbabbbd273c3ef29ef46e46",
			inInitiating:          true,
			inContents:            "5f67d15d22ca9b2804eeab0a66f7f8e3a10fa5de5809a046084348cbc5304e843ef96f59a59c7d7fdfe5946489f3ea297d941bac326225df316a25fc90f0e65b0d31a9c497e960fdbf8c482516bc8a9c1c77b7f6d0e1143810c737f76f9224e6f2c9af5186b4f7259c7e8d165b6e4fe3d38a60bdbdd4d06ecdcaaf62086070dbb68686b802d53dfd7db14b18743832605f5461ad81e2af4b7e8ff0eff0867a25b93cec7becf15c43131895fed09a83bf1ee4a87d44dd0f02a837bf5a1232e201cb882734eb9643dc2dc4d4e8b5690840766212c7ac8f38ad8a9ec47c7a9b3e022ae3eb6a32522128b518bd0d0085dd81c5",
			inMultiply:            69615,
			inAad:                 "",
			inIgnore:              true,
			midXOurs:              "8b8de966150bf872b4b695c9983df519c909811954d5d76e99ed0d5f1860247b",
			midXTheirs:            "eef379db9bd4b1aa90fc347fad33f7d53083389e22e971036f59f4e29d325ac2",
			midXShared:            "0a402d812314646ccc2565c315d1429ec1ed130ff92ff3f48d948f29c3762cf1",
			midSharedSecret:       "e25461fb0e4c162e18123ecde88342d54d449631e9b75a266fd9260c2bb2f41d",
			midInitiatorL:         "97771ce2ce17a25c3d65bf9f8e4acb830dce8d41392be3e4b8ed902a3106681a",
			midInitiatorP:         "2e7022b4eae9152942f68160a93e25d3e197a557385594aa587cb5e431bb470d",
			midResponderL:         "613f85a82d783ce450cfd7e91a027fcc4ad5610872f83e4dbe9e2202184c6d6e",
			midResponderP:         "cb5de4ed1083222e381401cf88e3167796bc9ab5b8aa1f27b718f39d1e6c0e87",
			midSendGarbageTerm:    "b709dea25e0be287c50e3603482c2e98",
			midRecvGarbageTerm:    "1f677e9d7392ebe3633fd82c9efb0f16",
			outSessionID:          "889f339285564fd868401fac8380bb9887925122ec8f31c8ae51ce067def103b",
			outCiphertext:         "",
			outCiphertextEndsWith: "7c4b9e1e6c1ce69da7b01513cdc4588fd93b04dafefaf87f31561763d906c672bac3dfceb751ebd126728ac017d4d580e931b8e5c7d5dfe0123be4dc9b2d2238b655c8a7fadaf8082c31e310909b5b731efc12f0a56e849eae6bfeedcc86dd27ef9b91d159256aa8e8d2b71a311f73350863d70f18d0d7302cf551e4303c7733",
		},
	}

	for _, test := range tests {
		inInitiating := test.inInitiating

		// We need to convert the FieldVal into a ModNScalar so that we
		// can use the ScalarBaseMultNonConst.
		inPrivOurs := setHex(test.inPrivOurs)
		inPrivOursBytes := inPrivOurs.Bytes()

		var inPrivOursScalar btcec.ModNScalar
		overflow := inPrivOursScalar.SetBytes(inPrivOursBytes)
		if overflow == 1 {
			t.Fatalf("unexpected reduction")
		}

		var inPubOurs btcec.JacobianPoint
		btcec.ScalarBaseMultNonConst(&inPrivOursScalar, &inPubOurs)
		inPubOurs.ToAffine()

		midXOurs := setHex(test.midXOurs)
		if !midXOurs.Equals(&inPubOurs.X) {
			t.Fatalf("expected mid-state to match our public key")
		}

		// ellswift_decode takes in ellswift_bytes and returns a proper key.
		// 1. convert from hex to bytes
		bytesEllswiftOurs, err := hex.DecodeString(test.inEllswiftOurs)
		if err != nil {
			t.Fatalf("unexpected error decoding string")
		}

		uEllswiftOurs := bytesEllswiftOurs[:32]
		tEllswiftOurs := bytesEllswiftOurs[32:]

		var (
			uEllswiftOursFV btcec.FieldVal
			tEllswiftOursFV btcec.FieldVal
		)

		truncated := uEllswiftOursFV.SetByteSlice(uEllswiftOurs)
		if truncated {
			uEllswiftOursFV.Normalize()
		}

		truncated = tEllswiftOursFV.SetByteSlice(tEllswiftOurs)
		if truncated {
			tEllswiftOursFV.Normalize()
		}

		xEllswiftOurs, err := ellswift.XSwiftEC(
			&uEllswiftOursFV, &tEllswiftOursFV,
		)
		if err != nil {
			t.Fatalf("unexpected error during XSwiftEC")
		}

		if !midXOurs.Equals(xEllswiftOurs) {
			t.Fatalf("expected mid-state to match decoded " +
				"ellswift key")
		}

		bytesEllswiftTheirs, err := hex.DecodeString(
			test.inEllswiftTheirs,
		)
		if err != nil {
			t.Fatalf("unexpected error decoding string")
		}

		uEllswiftTheirs := bytesEllswiftTheirs[:32]
		tEllswiftTheirs := bytesEllswiftTheirs[32:]

		var (
			uEllswiftTheirsFV btcec.FieldVal
			tEllswiftTheirsFV btcec.FieldVal
		)

		truncated = uEllswiftTheirsFV.SetByteSlice(uEllswiftTheirs)
		if truncated {
			uEllswiftTheirsFV.Normalize()
		}

		truncated = tEllswiftTheirsFV.SetByteSlice(tEllswiftTheirs)
		if truncated {
			tEllswiftTheirsFV.Normalize()
		}

		xEllswiftTheirs, err := ellswift.XSwiftEC(
			&uEllswiftTheirsFV, &tEllswiftTheirsFV,
		)
		if err != nil {
			t.Fatalf("unexpected error during XSwiftEC")
		}

		midXTheirs := setHex(test.midXTheirs)
		if !midXTheirs.Equals(xEllswiftTheirs) {
			t.Fatalf("expected mid-state to match decoded " +
				"ellswift key")
		}

		privKeyOurs, _ := btcec.PrivKeyFromBytes((*inPrivOursBytes)[:])

		var bytesEllswiftTheirs64 [64]byte
		copy(bytesEllswiftTheirs64[:], bytesEllswiftTheirs)

		xShared, err := ellswift.EllswiftECDHXOnly(
			bytesEllswiftTheirs64, privKeyOurs,
		)
		if err != nil {
			t.Fatalf("unexpected error when computing shared x")
		}

		var xSharedFV btcec.FieldVal
		overflow = xSharedFV.SetBytes(&xShared)
		if overflow == 1 {
			t.Fatalf("unexpected truncation")
		}

		midXShared := setHex(test.midXShared)

		if !midXShared.Equals(&xSharedFV) {
			t.Fatalf("expected mid-state x shared")
		}

		var bytesEllswiftOurs64 [64]byte
		copy(bytesEllswiftOurs64[:], bytesEllswiftOurs)

		sharedSecret, err := ellswift.V2Ecdh(
			privKeyOurs, bytesEllswiftTheirs64,
			bytesEllswiftOurs64, inInitiating,
		)
		if err != nil {
			t.Fatalf("unexpected error when calculating " +
				"shared secret")
		}

		midShared, err := hex.DecodeString(test.midSharedSecret)
		if err != nil {
			t.Fatalf("unexpected hex decode failure")
		}

		if !bytes.Equal(midShared, sharedSecret[:]) {
			t.Fatalf("expected mid shared secret")
		}

		p := NewPeer()

		buf := bytes.NewBuffer(nil)
		p.UseReadWriter(buf)

		err = p.createV2Ciphers(midShared, inInitiating, mainNet)
		if err != nil {
			t.Fatalf("error initiating v2 transport")
		}

		midInitiatorL, err := hex.DecodeString(test.midInitiatorL)
		if err != nil {
			t.Fatalf("unexpected error decoding midInitiatorL")
		}

		if !bytes.Equal(midInitiatorL, p.initiatorL) {
			t.Fatalf("expected mid-state initiatorL to " +
				"match computed value")
		}

		midInitiatorP, err := hex.DecodeString(test.midInitiatorP)
		if err != nil {
			t.Fatalf("unexpected error decoding midInitiatorP")
		}

		if !bytes.Equal(midInitiatorP, p.initiatorP) {
			t.Fatalf("expected mid-state initiatorP to " +
				"match computed value")
		}

		midResponderL, err := hex.DecodeString(test.midResponderL)
		if err != nil {
			t.Fatalf("unexpected error decoding midResponderL")
		}

		if !bytes.Equal(midResponderL, p.responderL) {
			t.Fatalf("expected mid-state responderL to " +
				"match computed value")
		}

		midResponderP, err := hex.DecodeString(test.midResponderP)
		if err != nil {
			t.Fatalf("unexpected error decoding midResponderP")
		}

		if !bytes.Equal(midResponderP, p.responderP) {
			t.Fatalf("expected mid-state responderP to " +
				"match computed value")
		}

		midSendGarbageTerm, err := hex.DecodeString(
			test.midSendGarbageTerm,
		)
		if err != nil {
			t.Fatalf("unexpected error decoding midSendGarbageTerm")
		}

		if !bytes.Equal(midSendGarbageTerm, p.sendGarbageTerm[:]) {
			t.Fatalf("expected mid-state sendGarbageTerm " +
				"to match computed value")
		}

		midRecvGarbageTerm, err := hex.DecodeString(
			test.midRecvGarbageTerm,
		)
		if err != nil {
			t.Fatalf("unexpected error decoding midRecvGarbageTerm")
		}

		if !bytes.Equal(midRecvGarbageTerm, p.recvGarbageTerm) {
			t.Fatalf("expected mid-state recvGarbageTerm to " +
				"match computed value")
		}

		outSessionID, err := hex.DecodeString(test.outSessionID)
		if err != nil {
			t.Fatalf("unexpected error decoding outSessionID")
		}

		if !bytes.Equal(outSessionID, p.sessionID) {
			t.Fatalf("expected sessionID to match computed value")
		}

		for i := 0; i < test.inIdx; i++ {
			_, _, err = p.V2EncPacket([]byte{}, []byte{}, false)
			if err != nil {
				t.Fatalf("unexpected error while encrypting packet")
			}
		}

		initialContents, err := hex.DecodeString(test.inContents)
		if err != nil {
			t.Fatalf("unexpected error decoding contents")
		}

		aad, err := hex.DecodeString(test.inAad)
		if err != nil {
			t.Fatalf("unexpected error decoding aad")
		}

		var contents []byte

		copy(contents, initialContents)

		for i := 0; i < test.inMultiply; i++ {
			contents = append(contents, initialContents...)
		}

		ciphertext, _, err := p.V2EncPacket(contents, aad, test.inIgnore)
		if err != nil {
			t.Fatalf("unexpected error when encrypting packet: %v", err)
		}

		if len(test.outCiphertext) != 0 {
			outCiphertextBytes, err := hex.DecodeString(test.outCiphertext)
			if err != nil {
				t.Fatalf("unexpected error decoding outCiphertext: %v", err)
			}

			if !bytes.Equal(outCiphertextBytes, ciphertext) {
				t.Fatalf("ciphertext mismatch")
			}
		}

		if len(test.outCiphertextEndsWith) != 0 {
			ciphertextHex := hex.EncodeToString(ciphertext)
			if !strings.HasSuffix(ciphertextHex, test.outCiphertextEndsWith) {
				t.Fatalf("suffix mismatch")
			}
		}
	}
}
