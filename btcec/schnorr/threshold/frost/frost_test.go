package frost

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path"
	"slices"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/threshold/simplpedpop"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/threshold/util"
	"github.com/stretchr/testify/require"
)

func TestSignAndVerifyRandom(ht *testing.T) {
	const iterations = 6

	for i := 0; i < iterations; i++ {
		n := RandIntForTest(ht, 9) + 2
		t := RandIntForTest(ht, n-1) + 2
		require.True(ht, t <= n)

		ht.Run(
			fmt.Sprintf("%d/%d of %d", i+1, t, n),
			func(ht *testing.T) {
				testSignAndVerify(ht, n, t)
			},
		)
	}
}

func testSignAndVerify(ht *testing.T, n, t int) {
	// Do a DKG
	var (
		seeds      = make([][32]byte, 0, n)
		pStates    = make([]*simplpedpop.ParticipantState, 0, n)
		pMsgs      = make([]*simplpedpop.ParticipantMessage, 0, n)
		pShares    = make([][]*btcec.ModNScalar, 0, n)
		dkgOutputs = make([]*util.DKGOutput, 0, n+1)
	)

	for i := 0; i < n; i++ {
		seed := Rand32IntForTest(ht)
		pState, pMsg, pShare, err := simplpedpop.ParticipantStep1(
			seed, t, n, i, Rand32IntForTest(ht),
		)
		require.NoError(ht, err)

		seeds = append(seeds, seed)
		pStates = append(pStates, pState)
		pShares = append(pShares, pShare)
		pMsgs = append(pMsgs, pMsg)
	}

	cMsg, cDkgOutput, _, err := simplpedpop.CoordinatorStep(pMsgs, t, n)
	require.NoError(ht, err)

	dkgOutputs = append(dkgOutputs, cDkgOutput)

	for i, pState := range pStates {
		partialShares := make([]*btcec.ModNScalar, 0, n)
		for _, share := range pShares {
			partialShares = append(partialShares, share[i])
		}

		secShare := simplpedpop.ParticipantStep2PrepareSecShare(
			partialShares,
		)

		dkgOutput, _, err := simplpedpop.ParticipantStep2(
			pState, cMsg, secShare,
		)
		require.NoError(ht, err)

		dkgOutputs = append(dkgOutputs, dkgOutput)
	}

	// Do a signing
	signerIds, signerShares := RandSignersForTest(
		ht, dkgOutputs[0].PubShares, t,
	)
	signerCount := len(signerIds)
	require.Equal(ht, signerCount, len(signerShares))
	require.True(ht, signerCount >= t)
	require.True(ht, signerCount <= n)

	threshPk := dkgOutputs[0].ThresholdPubKey
	require.NoError(ht, ValidateSessionParams(
		n, t, signerIds, signerShares, threshPk,
	))

	var (
		msg        = Rand32IntForTest(ht)
		msgBytes   = msg[:]
		v          = RandIntForTest(ht, 4)
		tweaks     = make([][32]byte, 0, v)
		tweakModes = make([]bool, 0, v)
	)

	for i := 0; i < v; i++ {
		// TODO(aakselrod): pre-reduce to avoid overflow flakes
		tweaks = append(tweaks, Rand32IntForTest(ht))
		tweakModes = append(tweakModes, (RandIntForTest(ht, 2) == 1))
	}

	secNonces := make([]*SecretNonce, 0, signerCount)
	pubNonces := make([]*PublicNonce, 0, signerCount)
	for i := range signerIds {
		secShare := dkgOutputs[signerIds[i]+1].SecShare
		require.True(ht, secShare.PubKey().IsEqual(signerShares[i]))

		extraIn := Rand32IntForTest(ht)
		secNonce, err := NonceGen(
			secShare, threshPk, &msgBytes, extraIn[:],
		)
		require.NoError(ht, err)
		secNonces = append(secNonces, secNonce)
		pubNonces = append(pubNonces, secNonce.PubNonce())
	}

	require.Equal(ht, signerCount, len(secNonces))
	require.Equal(ht, signerCount, len(pubNonces))

	sessionCtx := &SessionContext{
		N:               n,
		T:               t,
		Ids:             signerIds,
		PubShares:       signerShares,
		ThresholdPubKey: threshPk,
		AggNonce:        NonceAgg(pubNonces),
		Tweaks:          tweaks,
		IsXOnly:         tweakModes,
		Msg:             msgBytes,
	}
	pSigs := make([]*btcec.ModNScalar, 0, signerCount)
	for i := range signerIds {
		var (
			secShare        = dkgOutputs[signerIds[i]+1].SecShare
			secShareCleared bool
		)
		pSig, err := Sign(
			secNonces[i], secShare, signerIds[i], sessionCtx,
			func() error {
				secShareCleared = true
				return nil
			},
		)
		require.NoError(ht, err)
		require.True(ht, secShareCleared)
		require.True(ht, secNonces[i].Nonce1.Key.IsZero())
		require.True(ht, secNonces[i].Nonce2.Key.IsZero())
		require.True(ht, PartialSigVerify(
			pSig, pubNonces, n, t, signerIds, signerShares,
			threshPk, tweaks, tweakModes, msgBytes, i,
		))
		pSigs = append(pSigs, pSig)
	}

	sig, err := PartialSigAgg(pSigs, sessionCtx)
	require.NoError(ht, err)

	// Do a verify
	tCtx, err := ThresholdPubKeyAndTweak(threshPk, tweaks, tweakModes)
	require.NoError(ht, err)
	tweakedThresholdPubKey := util.GetXOnlyPubKey(tCtx.Q)

	require.True(ht, sig.Verify(msgBytes, tweakedThresholdPubKey))

}

const (
	signingTestVectorBaseDir             = "test_vectors"
	signingNonceGenTestVectorsFileName   = "nonce_gen_vectors.json"
	signingNonceAggTestVectorsFileName   = "nonce_agg_vectors.json"
	signingSignVerifyTestVectorsFileName = "sign_verify_vectors.json"
	signingTweakTestVectorsFileName      = "tweak_vectors.json"
	signingSigAggTestVectorsFileName     = "sig_agg_vectors.json"
	signingDetSignTestVectorsFileName    = "det_sign_vectors.json"
)

type signingNonceGenTestCase struct {
	TCID     int      `json:"tc_id"`
	Rand     string   `json:"rand"`
	SecShare *string  `json:"secshare"`
	PubShare *string  `json:"pubshare"`
	ThreshPK *string  `json:"thresh_pk_xonly"`
	Msg      *string  `json:"msg"`
	ExtraIn  *string  `json:"extra_in"`
	Expected []string `json:"expected"`
}

type signingNonceGenTestCases struct {
	TestCases []signingNonceGenTestCase `json:"valid_tests"`
}

func TestSigningNonceGenTestVectors(ht *testing.T) {
	testVectorPath := path.Join(
		signingTestVectorBaseDir, signingNonceGenTestVectorsFileName,
	)
	testVectorBytes, err := os.ReadFile(testVectorPath)
	require.NoError(ht, err)

	var testCases signingNonceGenTestCases
	require.NoError(ht, json.Unmarshal(testVectorBytes, &testCases))

	for _, testCase := range testCases.TestCases {
		var (
			random   [32]byte
			secShare *btcec.PrivateKey
			pubShare *btcec.PublicKey
			threshPK *btcec.PublicKey
			msg      *[]byte
			extraIn  []byte
		)

		copy(random[:], parseHex(ht, testCase.Rand))

		if testCase.SecShare != nil {
			//TODO(aakselrod): separate handling of pubshare
			secShare, pubShare = btcec.PrivKeyFromBytes(parseHex(
				ht, *testCase.SecShare,
			))

			if testCase.PubShare != nil {
				require.True(ht, pubShare.IsEqual(
					parseHexPubKey(ht, *testCase.PubShare),
				))
			}
		}

		if testCase.ThreshPK != nil {
			threshPK = parseHexXOnlyPubKey(ht, *testCase.ThreshPK)
		}

		if testCase.Msg != nil {
			msgBytes := parseHex(ht, *testCase.Msg)
			msg = &msgBytes
		}

		if testCase.ExtraIn != nil {
			extraIn = parseHex(ht, *testCase.ExtraIn)
		}

		ht.Run(fmt.Sprintf("%d", testCase.TCID), func(ht *testing.T) {
			secNonce, err := nonceGen(
				random, secShare, threshPK, msg, extraIn,
			)
			require.NoError(ht, err)
			require.Equal(
				ht, parseHex(ht, testCase.Expected[0]),
				secNonce.Bytes(),
			)
			require.Equal(
				ht, parseHex(ht, testCase.Expected[1]),
				secNonce.PubNonce().Bytes(),
			)
		})
	}
}

type frostExpectedErrorVector struct {
	Type        string  `json:"type"`
	Message     *string `json:"message"`
	SignerIndex *int    `json:"signer_index"`
	Contrib     *string `json:"contrib"`
}

func checkFROSTVectorError(ht *testing.T, expected *frostExpectedErrorVector,
	err error) {

	require.NotNil(ht, expected)
	require.Error(ht, err)

	var eMsg string
	if expected.Message != nil {
		eMsg = *expected.Message
		require.Equal(ht, eMsg, err.Error())
	}

	switch expected.Type {
	case "ValueError":
		require.ErrorContains(ht, err, eMsg)

	case "InvalidContributionError":
		require.NotNil(ht, expected.Contrib)
		var contrib ContributionType

		switch *expected.Contrib {
		case "pubnonce":
			contrib = ContributionTypePubNonce

		case "aggnonce":
			contrib = ContributionTypeAggNonce

		case "aggothernonce":
			contrib = ContributionTypeAggOtherNonce

		case "psig":
			contrib = ContributionTypePSig
		}

		idx := -1
		if expected.SignerIndex != nil {
			idx = *expected.SignerIndex
		}

		require.ErrorIs(ht, err, ErrInvalidContribution{idx, contrib})

	default:
		ht.Fatalf("Unknown expected error type: %s, got %+v",
			expected.Type, err)
	}
}

type signingNonceAggTestCase struct {
	TCID            int                       `json:"tc_id"`
	PubNonceIndices []int                     `json:"pubnonce_indices"`
	Expected        string                    `json:"expected"`
	Error           *frostExpectedErrorVector `json:"error"`
}

type signingNonceAggTestCases struct {
	PubNonces      []string                  `json:"pubnonces"`
	ValidTestCases []signingNonceAggTestCase `json:"valid_tests"`
	ErrorTestCases []signingNonceAggTestCase `json:"error_tests"`
}

func TestSigningNonceAggTestVectors(ht *testing.T) {
	testVectorPath := path.Join(
		signingTestVectorBaseDir, signingNonceAggTestVectorsFileName,
	)
	testVectorBytes, err := os.ReadFile(testVectorPath)
	require.NoError(ht, err)

	var testCases signingNonceAggTestCases
	require.NoError(ht, json.Unmarshal(testVectorBytes, &testCases))

	runTest := func(testCase signingNonceAggTestCase) (*PublicNonce,
		error) {

		nonces := make([]*PublicNonce, 0, len(testCase.PubNonceIndices))

		for i, nonceId := range testCase.PubNonceIndices {
			nonce, err := ParsePublicNonce(
				parseHex(ht, testCases.PubNonces[nonceId]),
			)
			if err != nil {
				return nil, ErrInvalidContribution{
					i, ContributionTypePubNonce,
				}
			}

			nonces = append(nonces, nonce)
		}

		return NonceAgg(nonces), nil
	}

	for _, testCase := range testCases.ValidTestCases {
		tcid := fmt.Sprintf("valid/%d", testCase.TCID)
		ht.Run(tcid, func(ht *testing.T) {
			aggNonce, err := runTest(testCase)
			require.NoError(ht, err)
			require.Equal(
				ht, parseHex(ht, testCase.Expected),
				aggNonce.Bytes(),
			)
		})
	}

	for _, testCase := range testCases.ErrorTestCases {
		tcid := fmt.Sprintf("error/%d", testCase.TCID)
		ht.Run(tcid, func(ht *testing.T) {
			aggNonce, err := runTest(testCase)
			require.Nil(ht, aggNonce)
			checkFROSTVectorError(ht, testCase.Error, err)
		})
	}
}

type signingSignVerifyTestCase struct {
	TCID            int                       `json:"tc_id"`
	MyId            int                       `json:"my_id"`
	Ids             []int                     `json:"ids"`
	PubShareIndices []int                     `json:"pubshare_indices"`
	PubNonceIndices []int                     `json:"pubnonce_indices"`
	SecShareIndex   int                       `json:"secshare_index"`
	SecNonceIndex   int                       `json:"secnonce_index"`
	AggNonce        string                    `json:"aggnonce"`
	SignerIndex     int                       `json:"signer_index"`
	PSig            string                    `json:"psig"`
	Msg             string                    `json:"msg"`
	Expected        *string                   `json:"expected"`
	Error           *frostExpectedErrorVector `json:"error"`
}

type signingSignVerifyTestGroup struct {
	TGID             string                      `json:"tg_id"`
	N                int                         `json:"n"`
	T                int                         `json:"t"`
	ThreshPK         string                      `json:"thresh_pk"`
	PubShares        []string                    `json:"pubshares"`
	PubNonces        []string                    `json:"pubnonces"`
	SecShares        []string                    `json:"secshares"`
	SecNonces        []string                    `json:"secnonces"`
	ValidTestCases   []signingSignVerifyTestCase `json:"valid_tests"`
	SignErrorTests   []signingSignVerifyTestCase `json:"sign_error_tests"`
	VerifyFailTests  []signingSignVerifyTestCase `json:"verify_fail_tests"`
	VerifyErrorTests []signingSignVerifyTestCase `json:"verify_error_tests"`
}

func TestSigningSignVerifyTestVectors(ht *testing.T) {
	testVectorPath := path.Join(
		signingTestVectorBaseDir, signingSignVerifyTestVectorsFileName,
	)
	testVectorBytes, err := os.ReadFile(testVectorPath)
	require.NoError(ht, err)

	var tests struct {
		TestGroups []signingSignVerifyTestGroup `json:"test_groups"`
	}
	require.NoError(ht, json.Unmarshal(testVectorBytes, &tests))

	parsePubInfo := func(tg signingSignVerifyTestGroup,
		testCase signingSignVerifyTestCase) (*btcec.PublicKey,
		[]*btcec.PublicKey, []*PublicNonce, []byte, error) {

		threshPK := parseHexPubKey(ht, tg.ThreshPK)

		pubShares := make(
			[]*btcec.PublicKey, 0,
			len(testCase.PubShareIndices),
		)
		for idx, i := range testCase.PubShareIndices {
			pubShare, err := btcec.ParsePubKey(
				parseHex(ht, tg.PubShares[i]),
			)
			if err != nil {
				return nil, nil, nil, nil, fmt.Errorf(
					"Invalid pubshare at index %d.", idx)
			}
			pubShares = append(pubShares, pubShare)
		}

		pubNonces := make(
			[]*PublicNonce, 0, len(testCase.PubNonceIndices),
		)
		for idx, i := range testCase.PubNonceIndices {
			pubNonce, err := ParsePublicNonce(
				parseHex(ht, tg.PubNonces[i]),
			)
			if err != nil {
				return nil, nil, nil, nil,
					ErrInvalidContribution{
						idx, ContributionTypePubNonce,
					}
			}
			pubNonces = append(pubNonces, pubNonce)
		}

		msg := parseHex(ht, testCase.Msg)

		return threshPK, pubShares, pubNonces, msg, nil
	}

	runSignTest := func(ht *testing.T, tg signingSignVerifyTestGroup,
		testCase signingSignVerifyTestCase) error {

		threshPK, pubShares, pubNonces, msg, err := parsePubInfo(
			tg, testCase,
		)
		if err != nil {
			return err
		}

		aggNonce := NonceAgg(pubNonces)
		testAggNonce, err := ParseAggNonce(
			parseHex(ht, testCase.AggNonce),
		)
		if err != nil {
			return err
		}

		sessionCtx := &SessionContext{
			N:               tg.N,
			T:               tg.T,
			Ids:             testCase.Ids,
			PubShares:       pubShares,
			ThresholdPubKey: threshPK,
			AggNonce:        testAggNonce,
			Msg:             parseHex(ht, testCase.Msg),
		}

		secNonce, err := ParseSecretNonce(
			parseHex(ht, tg.SecNonces[testCase.SecNonceIndex]),
		)
		if err != nil {
			return err
		}

		secShare, err := util.ParsePrivKeyNonZeroChecked(
			parseHex(ht, tg.SecShares[testCase.SecShareIndex]),
		)
		if err != nil {
			return fmt.Errorf("The signer's secret share value " +
				"is out of range.")
		}

		var secNonceCleared bool
		pSig, err := Sign(
			secNonce, secShare, testCase.MyId, sessionCtx,
			func() error {
				secNonceCleared = true
				return nil
			},
		)
		if err != nil {
			return err
		}
		require.True(ht, secNonceCleared)
		require.True(ht, secNonce.Nonce1.Key.IsZero())
		require.True(ht, secNonce.Nonce2.Key.IsZero())
		require.EqualValues(
			ht, parseHex(ht, *testCase.Expected), pSig.Bytes(),
		)
		require.EqualValues(ht, testAggNonce, aggNonce)

		// If we don't have the list of public shares, we can't verify
		// the partial signature, so we check that the test provides it
		// first.
		if len(pubShares) != 0 {
			require.True(ht, PartialSigVerify(
				pSig, pubNonces, tg.N, tg.T, testCase.Ids,
				pubShares, threshPK, nil, nil, msg,
				slices.Index(testCase.Ids, testCase.MyId),
			))
		}

		return nil
	}

	runVerifyTest := func(ht *testing.T, tg signingSignVerifyTestGroup,
		testCase signingSignVerifyTestCase) (bool, error) {

		threshPK, pubShares, pubNonces, msg, err := parsePubInfo(
			tg, testCase,
		)
		if err != nil {
			return false, err
		}

		var pSig = new(btcec.ModNScalar)
		pSig.SetByteSlice(parseHex(ht, testCase.PSig))

		return PartialSigVerify(
			pSig, pubNonces, tg.N, tg.T, testCase.Ids, pubShares,
			threshPK, nil, nil, msg,
			slices.Index(testCase.Ids, testCase.SignerIndex),
		), nil
	}

	for _, tg := range tests.TestGroups {
		ht.Run(tg.TGID, func(ht *testing.T) {
			for _, testCase := range tg.ValidTestCases {
				tcid := fmt.Sprintf(
					"valid/%d", testCase.TCID,
				)
				ht.Run(tcid, func(ht *testing.T) {
					err := runSignTest(ht, tg, testCase)
					require.NoError(ht, err)
				})
			}

			for _, testCase := range tg.SignErrorTests {
				tcid := fmt.Sprintf(
					"sign error/%d", testCase.TCID,
				)
				ht.Run(tcid, func(ht *testing.T) {
					err := runSignTest(ht, tg, testCase)
					checkFROSTVectorError(
						ht, testCase.Error, err,
					)
				})
			}

			for _, testCase := range tg.VerifyFailTests {
				tcid := fmt.Sprintf(
					"validate fail/%d", testCase.TCID,
				)
				ht.Run(tcid, func(ht *testing.T) {
					v, err := runVerifyTest(
						ht, tg, testCase,
					)
					require.NoError(ht, err)
					require.False(ht, v)
				})
			}

			for _, testCase := range tg.VerifyErrorTests {
				tcid := fmt.Sprintf(
					"validate error/%d", testCase.TCID,
				)
				ht.Run(tcid, func(ht *testing.T) {
					v, err := runVerifyTest(
						ht, tg, testCase,
					)
					require.False(ht, v)
					checkFROSTVectorError(
						ht, testCase.Error, err,
					)
				})
			}
		})
	}
}

type signingTweakTestCase struct {
	TCID            int                       `json:"tc_id"`
	MyId            int                       `json:"my_id"`
	Ids             []int                     `json:"ids"`
	PubShareIndices []int                     `json:"pubshare_indices"`
	PubNonceIndices []int                     `json:"pubnonce_indices"`
	SecShareIndex   int                       `json:"secshare_index"`
	SecNonceIndex   int                       `json:"secnonce_index"`
	AggNonce        string                    `json:"aggnonce"`
	Msg             string                    `json:"msg"`
	TweakIndices    []int                     `json:"tweak_indices"`
	IsXOnly         []bool                    `json:"is_xonly"`
	Expected        string                    `json:"expected"`
	Error           *frostExpectedErrorVector `json:"error"`
}

type signingTweakTestGroup struct {
	TGID           string                 `json:"tg_id"`
	N              int                    `json:"n"`
	T              int                    `json:"t"`
	ThreshPK       string                 `json:"thresh_pk"`
	PubShares      []string               `json:"pubshares"`
	PubNonces      []string               `json:"pubnonces"`
	SecShares      []string               `json:"secshares"`
	SecNonces      []string               `json:"secnonces"`
	Tweaks         []string               `json:"tweaks"`
	ValidTestCases []signingTweakTestCase `json:"valid_tests"`
	ErrorTestCases []signingTweakTestCase `json:"error_tests"`
}

func TestSigningTweakTestVectors(ht *testing.T) {
	testVectorPath := path.Join(
		signingTestVectorBaseDir, signingTweakTestVectorsFileName,
	)
	testVectorBytes, err := os.ReadFile(testVectorPath)
	require.NoError(ht, err)

	var tests struct {
		TestGroups []signingTweakTestGroup `json:"test_groups"`
	}
	require.NoError(ht, json.Unmarshal(testVectorBytes, &tests))

	runTest := func(ht *testing.T, tg signingTweakTestGroup,
		testCase signingTweakTestCase) error {

		threshPK := parseHexPubKey(ht, tg.ThreshPK)

		pubShares := make(
			[]*btcec.PublicKey, 0, len(testCase.PubShareIndices),
		)
		for _, i := range testCase.PubShareIndices {
			pubShares = append(pubShares, parseHexPubKey(
				ht, tg.PubShares[i],
			))
		}

		pubNonces := make(
			[]*PublicNonce, 0, len(testCase.PubNonceIndices),
		)
		for _, i := range testCase.PubNonceIndices {
			pubNonce, err := ParsePublicNonce(
				parseHex(ht, tg.PubNonces[i]),
			)
			if err != nil {
				return err
			}
			pubNonces = append(pubNonces, pubNonce)
		}

		tweaks := make([][32]byte, len(testCase.TweakIndices))
		for i, tweakIdx := range testCase.TweakIndices {
			tweak := parseHex(ht, tg.Tweaks[tweakIdx])
			if len(tweak) != 32 {
				return fmt.Errorf("The tweak must be a " +
					"32-byte array.")
			}

			copy(tweaks[i][:], tweak)
		}

		aggNonce := NonceAgg(pubNonces)
		testAggNonce, err := ParseAggNonce(
			parseHex(ht, testCase.AggNonce),
		)
		if err != nil {
			return err
		}

		sessionCtx := &SessionContext{
			N:               tg.N,
			T:               tg.T,
			Ids:             testCase.Ids,
			PubShares:       pubShares,
			ThresholdPubKey: threshPK,
			AggNonce:        aggNonce,
			Msg:             parseHex(ht, testCase.Msg),
			Tweaks:          tweaks,
			IsXOnly:         testCase.IsXOnly,
		}

		secNonce, err := ParseSecretNonce(
			parseHex(ht, tg.SecNonces[testCase.SecNonceIndex]),
		)
		if err != nil {
			return err
		}

		secShare, err := util.ParsePrivKeyNonZeroChecked(
			parseHex(ht, tg.SecShares[testCase.SecShareIndex]),
		)
		if err != nil {
			return fmt.Errorf("The signer's secret share value " +
				"is out of range.")
		}

		var secNonceCleared bool
		pSig, err := Sign(
			secNonce, secShare, testCase.MyId, sessionCtx,
			func() error {
				secNonceCleared = true
				return nil
			},
		)
		if err != nil {
			return err
		}
		require.True(ht, secNonceCleared)
		require.True(ht, secNonce.Nonce1.Key.IsZero())
		require.True(ht, secNonce.Nonce2.Key.IsZero())
		require.True(ht, PartialSigVerify(
			pSig, pubNonces, tg.N, tg.T, testCase.Ids, pubShares,
			threshPK, tweaks, testCase.IsXOnly,
			parseHex(ht, testCase.Msg),
			slices.Index(
				testCase.Ids, testCase.MyId,
			),
		))
		require.EqualValues(
			ht, parseHex(ht, testCase.Expected), pSig.Bytes(),
		)
		require.EqualValues(ht, testAggNonce, aggNonce)

		return nil
	}

	for _, tg := range tests.TestGroups {
		ht.Run(tg.TGID, func(ht *testing.T) {
			for _, testCase := range tg.ValidTestCases {
				tcid := fmt.Sprintf("valid/%d", testCase.TCID)
				ht.Run(tcid, func(ht *testing.T) {
					err := runTest(ht, tg, testCase)
					require.NoError(ht, err)
				})
			}

			for _, testCase := range tg.ErrorTestCases {
				tcid := fmt.Sprintf("error/%d", testCase.TCID)
				ht.Run(tcid, func(ht *testing.T) {
					err := runTest(ht, tg, testCase)
					checkFROSTVectorError(
						ht, testCase.Error, err,
					)
				})
			}
		})
	}
}

type signingSigAggTestCase struct {
	TCID            int                       `json:"tc_id"`
	Ids             []int                     `json:"ids"`
	PubShareIndices []int                     `json:"pubshare_indices"`
	AggNonce        string                    `json:"aggnonce"`
	TweakIndices    []int                     `json:"tweak_indices"`
	IsXOnly         []bool                    `json:"is_xonly"`
	PSigs           []string                  `json:"psigs"`
	Msg             string                    `json:"msg"`
	Expected        string                    `json:"expected"`
	Error           *frostExpectedErrorVector `json:"error"`
}

type signingSigAggTestGroup struct {
	TGID           string                  `json:"tg_id"`
	N              int                     `json:"n"`
	T              int                     `json:"t"`
	ThreshPK       string                  `json:"thresh_pk"`
	PubShares      []string                `json:"pubshares"`
	Tweaks         []string                `json:"tweaks"`
	ValidTestCases []signingSigAggTestCase `json:"valid_tests"`
	ErrorTestCases []signingSigAggTestCase `json:"error_tests"`
}

func TestSigningSigAggTestVectors(ht *testing.T) {
	testVectorPath := path.Join(
		signingTestVectorBaseDir, signingSigAggTestVectorsFileName,
	)
	testVectorBytes, err := os.ReadFile(testVectorPath)
	require.NoError(ht, err)

	var tests struct {
		TestGroups []signingSigAggTestGroup `json:"test_groups"`
	}
	require.NoError(ht, json.Unmarshal(testVectorBytes, &tests))

	runTest := func(ht *testing.T, tg signingSigAggTestGroup,
		testCase signingSigAggTestCase) error {

		threshPK := parseHexPubKey(ht, tg.ThreshPK)

		pubShares := make(
			[]*btcec.PublicKey, 0, len(testCase.PubShareIndices),
		)
		for _, i := range testCase.PubShareIndices {
			pubShares = append(
				pubShares, parseHexPubKey(ht, tg.PubShares[i]),
			)
		}

		aggNonce, err := ParseAggNonce(parseHex(ht, testCase.AggNonce))
		if err != nil {
			return err
		}

		tweaks := make([][32]byte, len(testCase.TweakIndices))
		for i, tweakIdx := range testCase.TweakIndices {
			tweak := parseHex(ht, tg.Tweaks[tweakIdx])
			if len(tweak) != 32 {
				return fmt.Errorf("The tweak must be a " +
					"32-byte array.")
			}

			copy(tweaks[i][:], tweak)
		}

		require.Equal(ht, len(tweaks), len(testCase.IsXOnly))

		pSigs := make([]*btcec.ModNScalar, 0, len(testCase.PSigs))
		for i, pSigHex := range testCase.PSigs {
			pSig := new(btcec.ModNScalar)
			overflows := pSig.SetByteSlice(parseHex(ht, pSigHex))
			if overflows {
				return ErrInvalidContribution{
					i, ContributionTypePSig,
				}
			}

			pSigs = append(pSigs, pSig)
		}

		expected := parseHex(ht, testCase.Expected)

		msg := parseHex(ht, testCase.Msg)

		sessionCtx := &SessionContext{
			N:               tg.N,
			T:               tg.T,
			Ids:             testCase.Ids,
			PubShares:       pubShares,
			ThresholdPubKey: threshPK,
			AggNonce:        aggNonce,
			Tweaks:          tweaks,
			IsXOnly:         testCase.IsXOnly,
			Msg:             msg,
		}

		sig, err := PartialSigAgg(pSigs, sessionCtx)
		if err != nil {
			return err
		}
		require.Equal(ht, expected, sig.Serialize())

		tweakCtx, err := ThresholdPubKeyAndTweak(
			threshPK, tweaks, testCase.IsXOnly,
		)
		if err != nil {
			return err
		}

		require.True(ht, sig.Verify(
			msg, util.GetXOnlyPubKey(tweakCtx.Q),
		))

		return nil
	}

	for _, tg := range tests.TestGroups {
		ht.Run(tg.TGID, func(ht *testing.T) {
			for _, testCase := range tg.ValidTestCases {
				tcid := fmt.Sprintf("valid/%d", testCase.TCID)
				ht.Run(tcid, func(ht *testing.T) {
					err := runTest(ht, tg, testCase)
					require.NoError(ht, err)
				})
			}

			for _, testCase := range tg.ErrorTestCases {
				tcid := fmt.Sprintf("error/%d", testCase.TCID)
				ht.Run(tcid, func(ht *testing.T) {
					err := runTest(ht, tg, testCase)
					checkFROSTVectorError(
						ht, testCase.Error, err,
					)
				})
			}
		})
	}
}

type signingDetSignTestCase struct {
	TCID            int                       `json:"tc_id"`
	MyId            int                       `json:"my_id"`
	Ids             []int                     `json:"ids"`
	PubShareIndices []int                     `json:"pubshare_indices"`
	SecShareIndex   int                       `json:"secshare_index"`
	AggOtherNonce   string                    `json:"aggothernonce"`
	AuxRand         string                    `json:"aux_rand"`
	Msg             string                    `json:"msg"`
	Tweaks          []string                  `json:"tweaks"`
	IsXOnly         []bool                    `json:"is_xonly"`
	Expected        []string                  `json:"expected"`
	Error           *frostExpectedErrorVector `json:"error"`
}

type signingDetSignTestGroup struct {
	TGID           string                   `json:"tg_id"`
	N              int                      `json:"n"`
	T              int                      `json:"t"`
	ThreshPK       string                   `json:"thresh_pk"`
	PubShares      []string                 `json:"pubshares"`
	SecShares      []string                 `json:"secshares"`
	ValidTestCases []signingDetSignTestCase `json:"valid_tests"`
	ErrorTestCases []signingDetSignTestCase `json:"error_tests"`
}

func TestSigningDetSignTestVectors(ht *testing.T) {
	testVectorPath := path.Join(
		signingTestVectorBaseDir, signingDetSignTestVectorsFileName,
	)
	testVectorBytes, err := os.ReadFile(testVectorPath)
	require.NoError(ht, err)

	var tests struct {
		TestGroups []signingDetSignTestGroup `json:"test_groups"`
	}
	require.NoError(ht, json.Unmarshal(testVectorBytes, &tests))

	runTest := func(ht *testing.T, tg signingDetSignTestGroup,
		testCase signingDetSignTestCase) (*PublicNonce,
		*btcec.ModNScalar, error) {

		threshPK := parseHexPubKey(ht, tg.ThreshPK)

		pubShares := make(
			[]*btcec.PublicKey, 0, len(testCase.PubShareIndices),
		)
		for idx, i := range testCase.PubShareIndices {
			pubShare, err := btcec.ParsePubKey(
				parseHex(ht, tg.PubShares[i]),
			)
			if err != nil {
				return nil, nil, fmt.Errorf(
					"Invalid pubshare at index %d.", idx)
			}
			pubShares = append(pubShares, pubShare)
		}

		secShare, err := util.ParsePrivKeyNonZeroChecked(
			parseHex(ht, tg.SecShares[testCase.SecShareIndex]),
		)
		if err != nil {
			return nil, nil, fmt.Errorf("The signer's secret " +
				"share value is out of range.")
		}

		tweaks := make([][32]byte, len(testCase.Tweaks))
		for i, tweakHex := range testCase.Tweaks {
			tweak := parseHex(ht, tweakHex)
			if len(tweak) != 32 {
				return nil, nil, fmt.Errorf("The tweak must " +
					"be a 32-byte array.")
			}

			copy(tweaks[i][:], tweak)
		}

		var aggOtherNonce *PublicNonce
		if len(testCase.AggOtherNonce) != 0 {
			aggOtherNonce, err = ParsePublicNonce(
				parseHex(ht, testCase.AggOtherNonce),
			)
			if err != nil {
				return nil, nil, ErrInvalidContribution{
					-1, ContributionTypeAggOtherNonce,
				}
			}
		}

		msg := parseHex(ht, testCase.Msg)

		auxRand := parseHex(ht, testCase.AuxRand)

		pubNonce, pSig, err := DeterministicSign(
			secShare, testCase.MyId, aggOtherNonce, tg.N, tg.T,
			testCase.Ids, pubShares, threshPK, tweaks,
			testCase.IsXOnly, msg, auxRand,
		)
		if err != nil {
			return nil, nil, err
		}

		aggNonce := pubNonce
		if aggOtherNonce != nil {
			aggNonce = NonceAgg(
				[]*PublicNonce{aggOtherNonce, pubNonce},
			)
		}

		myPubShare, err := btcec.ParsePubKey(parseHex(
			ht, tg.PubShares[testCase.MyId],
		))
		if err != nil {
			return nil, nil, err
		}
		require.Equal(ht, myPubShare, secShare.PubKey())

		sessionCtx := &SessionContext{
			N:               tg.N,
			T:               tg.T,
			Ids:             testCase.Ids,
			PubShares:       pubShares,
			ThresholdPubKey: threshPK,
			AggNonce:        aggNonce,
			Tweaks:          tweaks,
			IsXOnly:         testCase.IsXOnly,
			Msg:             msg,
		}

		require.True(ht, partialSigVerify(
			pSig, testCase.MyId, pubNonce, myPubShare, sessionCtx,
		))

		return pubNonce, pSig, nil
	}

	for _, tg := range tests.TestGroups {
		ht.Run(tg.TGID, func(ht *testing.T) {
			for _, testCase := range tg.ValidTestCases {
				tcid := fmt.Sprintf("valid/%d", testCase.TCID)
				ht.Run(tcid, func(ht *testing.T) {
					pubNonce, pSig, err := runTest(
						ht, tg, testCase,
					)
					require.NoError(ht, err)
					require.Equal(
						ht, parseHex(
							ht,
							testCase.Expected[0],
						), pubNonce.Bytes(),
					)
					require.EqualValues(
						ht, parseHex(
							ht,
							testCase.Expected[1],
						), pSig.Bytes(),
					)
				})
			}

			for _, testCase := range tg.ErrorTestCases {
				tcid := fmt.Sprintf("error/%d", testCase.TCID)
				ht.Run(tcid, func(ht *testing.T) {
					pubNonce, pSig, err := runTest(
						ht, tg, testCase,
					)
					require.Nil(ht, pubNonce)
					require.Nil(ht, pSig)
					checkFROSTVectorError(
						ht, testCase.Error, err,
					)
				})
			}
		})
	}
}

func RandIntForTest(ht *testing.T, maxValue int) int {
	randInt, err := rand.Int(
		rand.Reader, big.NewInt(int64(maxValue)),
	)
	require.NoError(ht, err)
	return int(randInt.Int64())
}

// TODO(aakselrod): change to pointer to avoid copies
func Rand32IntForTest(ht *testing.T) [32]byte {
	random, err := Rand32Int()
	require.NoError(ht, err)
	return *random
}

func RandSignersForTest(ht *testing.T, pubshares []*btcec.PublicKey, t int) (
	[]int, []*btcec.PublicKey) {

	n := len(pubshares)
	require.True(ht, t <= n)

	numSigners := RandIntForTest(ht, n-t+1) + t

	ids := make([]int, 0, n)
	for i := 0; i < n; i++ {
		ids = append(ids, i)
	}
	for i := 0; i < n-numSigners; i++ {
		idx := RandIntForTest(ht, len(ids))
		ids = slices.Delete(ids, idx, idx+1)
	}

	require.Equal(ht, numSigners, len(ids))

	signerShares := make([]*btcec.PublicKey, 0, numSigners)

	for _, id := range ids {
		signerShares = append(signerShares, pubshares[id])
	}

	return ids, signerShares
}

func parseHexXOnlyPubKey(ht *testing.T, str string) *btcec.PublicKey {
	key, err := schnorr.ParsePubKey(parseHex(ht, str))
	require.NoError(ht, err)

	return key
}

func parseHexPubKey(ht *testing.T, str string) *btcec.PublicKey {
	key, err := btcec.ParsePubKey(parseHex(ht, str))
	require.NoError(ht, err)

	return key
}

func parseHex(ht *testing.T, msg string) []byte {
	b, err := hex.DecodeString(msg)
	require.NoError(ht, err)

	return b
}
