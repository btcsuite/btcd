package chilldkg

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/threshold"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/stretchr/testify/require"
)

const (
	dkgTestVectorBaseDir                     = "test_vectors"
	dkgHostPubKeyGenVectorsFileName          = "hostpubkey_gen_vectors.json"
	dkgParamsHashVectorsFileName             = "params_hash_vectors.json"
	dkgParticipantStep1VectorsFileName       = "participant_step1_vectors.json"
	dkgParticipantStep2VectorsFileName       = "participant_step2_vectors.json"
	dkgParticipantFinalizeVectorsFileName    = "participant_finalize_vectors.json"
	dkgParticipantInvestigateVectorsFileName = "participant_investigate_vectors.json"
	dkgCoordinatorStep1VectorsFileName       = "coordinator_step1_vectors.json"
	dkgCoordinatorFinalizeVectorsFileName    = "coordinator_finalize_vectors.json"
	dkgCoordinatorInvestigateVectorsFileName = "coordinator_investigate_vectors.json"
	dkgRecoverVectorsFileName                = "recover_vectors.json"
)

type dkgExpectedErrorVector struct {
	Type         string  `json:"type"`
	Message      *string `json:"message"`
	Participant  *int    `json:"participantId"`
	Participant1 *int    `json:"participantId1"`
	Participant2 *int    `json:"participantId2"`
}

var (
	errRandomness = errors.New("Randomness length is not 32 bytes")
)

func checkDkgVectorError(ht *testing.T, expected *dkgExpectedErrorVector,
	err error) {

	require.NotNil(ht, expected)
	require.Error(ht, err)

	var eMsg string
	if expected.Message != nil {
		eMsg = *expected.Message
		require.Equal(ht, eMsg, err.Error())
	}

	switch expected.Type {
	case "HostSeckeyError":
		require.ErrorIs(ht, err, ErrHostSecKey(eMsg))

	case "ThresholdOrCountError":
		require.ErrorIs(ht, err, ErrThresholdOrCount)

	case "InvalidHostPubkeyError":
		require.NotNil(ht, expected.Participant)
		require.ErrorIs(
			ht, err, errInvalidHostPubKey(*expected.Participant),
		)

	case "ValueError":
		//TODO(aakselrod): revisit after generic error improved in spec
		require.Error(ht, err)

	case "DuplicateHostPubkeyError":
		require.NotNil(ht, expected.Participant1)
		require.NotNil(ht, expected.Participant2)
		require.ErrorIs(ht, err, ErrDuplicateHostPubKey{
			*expected.Participant1, *expected.Participant2,
		})

	case "RandomnessError":
		require.ErrorIs(ht, err, errRandomness)

	case "FaultyCoordinatorError":
		require.ErrorIs(ht, err, threshold.ErrFaultyCoordinator(eMsg))

	case "RecoveryDataError":
		require.ErrorIs(ht, err, ErrRecoveryData(eMsg))

	case "FaultyParticipantOrCoordinatorError":
		require.ErrorIs(ht, err, threshold.ErrFaultyParticipantOrCoordinator{
			eMsg, *expected.Participant,
		})

	case "FaultyParticipantError":
		require.ErrorIs(ht, err, threshold.ErrFaultyParticipant{
			eMsg, *expected.Participant,
		})

	case "UnknownFaultyParticipantOrCoordinatorError":
		mErr, ok := err.(ErrUnknownFaultyParticipantOrCoordinator)
		require.True(ht, ok)
		require.NotNil(ht, mErr.InvData)

	default:
		ht.Fatalf("Unknown expected error type: %s, got %+v",
			expected.Type, err)
	}
}

type dkgHostPubKeyGenTestCase struct {
	TCID               int                     `json:"tcId"`
	HostSecKey         string                  `json:"hostseckey"`
	ExpectedHostPubKey *string                 `json:"expectedHostpubkey"`
	ExpectedError      *dkgExpectedErrorVector `json:"expectedError"`
}

type dkgHostPubKeyGenTestCases struct {
	ValidTestCases []dkgHostPubKeyGenTestCase `json:"validTestCases"`
	ErrorTestCases []dkgHostPubKeyGenTestCase `json:"errorTestCases"`
}

func TestVectorsDKGHostPubKeyGen(ht *testing.T) {
	testVectorPath := path.Join(
		dkgTestVectorBaseDir, dkgHostPubKeyGenVectorsFileName,
	)
	testVectorBytes, err := os.ReadFile(testVectorPath)
	require.NoError(ht, err)

	var testCases dkgHostPubKeyGenTestCases
	require.NoError(ht, json.Unmarshal(testVectorBytes, &testCases))

	runTest := func(testCase dkgHostPubKeyGenTestCase) (*btcec.PublicKey,
		error) {

		_, pubKey, err := hostPubKeyGen(
			parseHex(testCase.HostSecKey),
		)

		return pubKey, err
	}

	for _, testCase := range testCases.ValidTestCases {
		tcid := fmt.Sprintf("valid/%d", testCase.TCID)
		ht.Run(tcid, func(ht *testing.T) {
			res, err := runTest(testCase)
			require.NoError(ht, err)
			require.Equal(
				ht, parseHex(*testCase.ExpectedHostPubKey),
				res.SerializeCompressed(),
			)
		})
	}

	for _, testCase := range testCases.ErrorTestCases {
		tcid := fmt.Sprintf("error/%d", testCase.TCID)
		ht.Run(tcid, func(ht *testing.T) {
			res, err := runTest(testCase)
			require.Nil(ht, res)
			checkDkgVectorError(ht, testCase.ExpectedError, err)
		})
	}
}

type dkgParamsVector struct {
	HostPubKeys []string `json:"hostpubkeys"`
	T           int      `json:"t"`
}

type dkgParamsHashTestCase struct {
	TCID               int                     `json:"tcId"`
	Params             dkgParamsVector         `json:"params"`
	ExpectedParamsHash *string                 `json:"expectedParamsHash"`
	ExpectedError      *dkgExpectedErrorVector `json:"expectedError"`
}

type dkgParamsHashTestCases struct {
	ValidTestCases []dkgParamsHashTestCase `json:"validTestCases"`
	ErrorTestCases []dkgParamsHashTestCase `json:"errorTestCases"`
}

func TestVectorsDKGParamsHash(ht *testing.T) {
	testVectorPath := path.Join(
		dkgTestVectorBaseDir, dkgParamsHashVectorsFileName,
	)
	testVectorBytes, err := os.ReadFile(testVectorPath)
	require.NoError(ht, err)

	var testCases dkgParamsHashTestCases
	require.NoError(ht, json.Unmarshal(testVectorBytes, &testCases))

	runTest := func(testCase dkgParamsHashTestCase) (*chainhash.Hash,
		error) {

		params, err := readParamsVector(testCase.Params)
		if err != nil {
			return nil, err
		}

		return ParamsHash(params)
	}

	for _, testCase := range testCases.ValidTestCases {
		tcid := fmt.Sprintf("valid/%d", testCase.TCID)
		ht.Run(tcid, func(ht *testing.T) {
			res, err := runTest(testCase)
			require.NoError(ht, err)
			require.Equal(
				ht, parseHex(*testCase.ExpectedParamsHash),
				res[:],
			)
		})
	}

	for _, testCase := range testCases.ErrorTestCases {
		tcid := fmt.Sprintf("error/%d", testCase.TCID)
		ht.Run(tcid, func(ht *testing.T) {
			res, err := runTest(testCase)
			require.Nil(ht, res)
			checkDkgVectorError(ht, testCase.ExpectedError, err)
		})
	}
}

type dkgParticipantStep1TestCase struct {
	TCID          int                     `json:"tcId"`
	HostSecKey    string                  `json:"hostseckey"`
	Params        dkgParamsVector         `json:"params"`
	Random        string                  `json:"random"`
	ExpectedPmsg1 *string                 `json:"expectedPmsg1"`
	ExpectedError *dkgExpectedErrorVector `json:"expectedError"`
}

type dkgParticipantStep1TestGroup struct {
	ValidTestCases []dkgParticipantStep1TestCase `json:"validTestCases"`
	ErrorTestCases []dkgParticipantStep1TestCase `json:"errorTestCases"`
}

func TestVectorsDKGParticipantStep1(ht *testing.T) {
	testVectorPath := path.Join(
		dkgTestVectorBaseDir, dkgParticipantStep1VectorsFileName,
	)
	testVectorBytes, err := os.ReadFile(testVectorPath)
	require.NoError(ht, err)

	var tests struct {
		TestGroups []dkgParticipantStep1TestGroup `json:"testGroups"`
	}
	require.NoError(ht, json.Unmarshal(testVectorBytes, &tests))

	var allZeroes [32]byte

	runTest := func(testCase dkgParticipantStep1TestCase) (
		*ParticipantMsg1, error) {

		params, err := readParamsVector(testCase.Params)
		if err != nil {
			return nil, err
		}

		hostSecKey, _, err := hostPubKeyGen(
			parseHex(testCase.HostSecKey),
		)
		if err != nil {
			return nil, err
		}

		randBytes := parseHex(testCase.Random)
		if len(randBytes) != 32 || bytes.Equal(
			randBytes, allZeroes[:],
		) {

			return nil, errRandomness
		}

		var random [32]byte
		copy(random[:], randBytes)

		_, pmsg1, err := ParticipantStep1(hostSecKey, params, &random)
		return pmsg1, err
	}

	for _, tg := range tests.TestGroups {
		for _, testCase := range tg.ValidTestCases {
			tcid := fmt.Sprintf("valid/%d", testCase.TCID)
			ht.Run(tcid, func(ht *testing.T) {
				res, err := runTest(testCase)
				require.NoError(ht, err)
				require.Equal(
					ht, parseHex(*testCase.ExpectedPmsg1),
					res.Bytes(),
				)
			})
		}

		for _, testCase := range tg.ErrorTestCases {
			tcid := fmt.Sprintf("error/%d", testCase.TCID)
			ht.Run(tcid, func(ht *testing.T) {
				res, err := runTest(testCase)
				require.Nil(ht, res)
				checkDkgVectorError(
					ht, testCase.ExpectedError, err,
				)
			})
		}
	}
}

type dkgParticipantStep2TestCase struct {
	TCID          int                     `json:"tcId"`
	Cmsg1         string                  `json:"cmsg1"`
	HostSecKey    *string                 `json:"hostseckey"`
	AuxRand       *string                 `json:"auxRand"`
	ExpectedPmsg2 *string                 `json:"expectedPmsg2"`
	ExpectedError *dkgExpectedErrorVector `json:"expectedError"`
}

type dkgParticipantStep2TestGroup struct {
	Params         dkgParamsVector               `json:"params"`
	HostSecKey     string                        `json:"hostseckey"`
	Random         string                        `json:"random"`
	AuxRand        string                        `json:"auxRand"`
	Pmsg1          string                        `json:"pmsg1"`
	ValidTestCases []dkgParticipantStep2TestCase `json:"validTestCases"`
	ErrorTestCases []dkgParticipantStep2TestCase `json:"errorTestCases"`
}

func TestVectorsDKGParticipantStep2(ht *testing.T) {
	testVectorPath := path.Join(
		dkgTestVectorBaseDir, dkgParticipantStep2VectorsFileName,
	)
	testVectorBytes, err := os.ReadFile(testVectorPath)
	require.NoError(ht, err)

	var tests struct {
		TestGroups []dkgParticipantStep2TestGroup `json:"testGroups"`
	}
	require.NoError(ht, json.Unmarshal(testVectorBytes, &tests))

	runTest := func(tg dkgParticipantStep2TestGroup,
		testCase dkgParticipantStep2TestCase) (*ParticipantMsg2,
		error) {

		params := mustReadParamsVector(tg.Params)

		hostSecKey, _, err := hostPubKeyGen(parseHex(tg.HostSecKey))
		if err != nil {
			return nil, err
		}

		var random, auxRand [32]byte
		copy(random[:], parseHex(tg.Random))
		copy(auxRand[:], parseHex(tg.AuxRand))

		if testCase.AuxRand != nil {
			auxRandBytes := parseHex(*testCase.AuxRand)
			if len(auxRandBytes) != 32 {
				return nil, errors.New("invalid randomness " +
					"length")
			}
			copy(auxRand[:], auxRandBytes)
		}

		pstate1, pmsg1, err := ParticipantStep1(
			hostSecKey, params, &random,
		)
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(parseHex(tg.Pmsg1), pmsg1.Bytes()) {
			return nil, errors.New("participant msg1 not equal " +
				"to expected")
		}

		cmsg1, err := ParseCoordinatorMsg1(
			parseHex(testCase.Cmsg1), params.T,
			len(params.HostPubKeys),
		)
		if err != nil {
			if fpcErr, ok := err.(threshold.
				ErrFaultyParticipantOrCoordinator); ok &&
				fpcErr.Participant == pstate1.Idx {

				return nil, threshold.ErrFaultyCoordinator(
					"Coordinator replied with wrong " +
						"pubnonce")
			}

			return nil, err
		}

		if testCase.HostSecKey != nil {
			hostSecKey, _, err = hostPubKeyGen(
				parseHex(*testCase.HostSecKey),
			)
			if err != nil {
				return nil, err
			}
		}

		_, pmsg2, err := ParticipantStep2(
			hostSecKey, pstate1, cmsg1, &auxRand,
		)
		return pmsg2, err
	}

	for _, tg := range tests.TestGroups {
		for _, testCase := range tg.ValidTestCases {
			tcid := fmt.Sprintf("valid/%d", testCase.TCID)
			ht.Run(tcid, func(ht *testing.T) {
				res, err := runTest(tg, testCase)
				require.NoError(ht, err)
				require.Equal(
					ht, parseHex(*testCase.ExpectedPmsg2),
					res.Bytes(),
				)
			})
		}

		for _, testCase := range tg.ErrorTestCases {
			tcid := fmt.Sprintf("error/%d", testCase.TCID)
			ht.Run(tcid, func(ht *testing.T) {
				res, err := runTest(tg, testCase)
				require.Nil(ht, res)
				checkDkgVectorError(
					ht, testCase.ExpectedError, err,
				)
			})
		}
	}
}

type dkgOutputVector struct {
	SecShare        *string  `json:"secshare"`
	ThresholdPubKey string   `json:"threshPk"`
	PubShares       []string `json:"pubshares"`
}

type dkgParticipantFinalizeExpectedOutput struct {
	DKGOutput    dkgOutputVector `json:"dkgOutput"`
	RecoveryData string          `json:"recoveryData"`
}

type dkgParticipantFinalizeTestCase struct {
	TCID           int                                   `json:"tcId"`
	Cmsg2          string                                `json:"cmsg2"`
	ExpectedOutput *dkgParticipantFinalizeExpectedOutput `json:"expectedOutput"`
	ExpectedError  *dkgExpectedErrorVector               `json:"expectedError"`
}

type dkgParticipantFinalizeTestGroup struct {
	Params         dkgParamsVector                  `json:"params"`
	HostSecKey     string                           `json:"hostseckey"`
	Random         string                           `json:"random"`
	AuxRand        string                           `json:"auxRand"`
	Pmsg1          string                           `json:"pmsg1"`
	Cmsg1          string                           `json:"cmsg1"`
	Pmsg2          string                           `json:"pmsg2"`
	ValidTestCases []dkgParticipantFinalizeTestCase `json:"validTestCases"`
	ErrorTestCases []dkgParticipantFinalizeTestCase `json:"errorTestCases"`
}

func TestVectorsDKGParticipantFinalize(ht *testing.T) {
	testVectorPath := path.Join(
		dkgTestVectorBaseDir, dkgParticipantFinalizeVectorsFileName,
	)
	testVectorBytes, err := os.ReadFile(testVectorPath)
	require.NoError(ht, err)

	var tests struct {
		TestGroups []dkgParticipantFinalizeTestGroup `json:"testGroups"`
	}
	require.NoError(ht, json.Unmarshal(testVectorBytes, &tests))

	runTest := func(tg dkgParticipantFinalizeTestGroup,
		testCase dkgParticipantFinalizeTestCase) (*threshold.DKGOutput,
		*RecoveryData, error) {

		params := mustReadParamsVector(tg.Params)

		hostSecKeyBytes := parseHex(tg.HostSecKey)
		hostSecKey, _ := btcec.PrivKeyFromBytes(hostSecKeyBytes)

		var random, auxRand [32]byte
		copy(random[:], parseHex(tg.Random))
		copy(auxRand[:], parseHex(tg.AuxRand))

		pstate1, pmsg1, err := ParticipantStep1(
			hostSecKey, params, &random,
		)
		if err != nil {
			return nil, nil, err
		}
		if !bytes.Equal(parseHex(tg.Pmsg1), pmsg1.Bytes()) {
			return nil, nil, errors.New("participant msg1 not " +
				"equal to expected")
		}

		cmsg1, err := ParseCoordinatorMsg1(
			parseHex(tg.Cmsg1), params.T,
			len(params.HostPubKeys),
		)
		if err != nil {
			return nil, nil, err
		}

		pstate2, pmsg2, err := ParticipantStep2(
			hostSecKey, pstate1, cmsg1, &auxRand,
		)
		if err != nil {
			return nil, nil, err
		}
		if !bytes.Equal(parseHex(tg.Pmsg2), pmsg2.Bytes()) {
			return nil, nil, errors.New("participant msg2 not " +
				"equal to expected")
		}

		cmsg2, err := ParseCoordinatorMsg2(
			parseHex(testCase.Cmsg2),
		)
		if err != nil {
			return nil, nil, err
		}

		return ParticipantFinalize(pstate2, cmsg2)
	}

	for _, tg := range tests.TestGroups {
		for _, testCase := range tg.ValidTestCases {
			tcid := fmt.Sprintf("valid/%d", testCase.TCID)
			ht.Run(tcid, func(ht *testing.T) {
				dkgOutput, recData, err := runTest(tg, testCase)
				require.NoError(ht, err)
				require.Equal(
					ht,
					testCase.ExpectedOutput.RecoveryData,
					fmt.Sprintf("%X", *recData),
				)
				require.Equal(ht, parseDkgOutputVector(
					ht, testCase.ExpectedOutput.DKGOutput,
				), dkgOutput)
			})
		}

		for _, testCase := range tg.ErrorTestCases {
			tcid := fmt.Sprintf("error/%d", testCase.TCID)
			ht.Run(tcid, func(ht *testing.T) {
				res1, res2, err := runTest(tg, testCase)
				require.Nil(ht, res1)
				require.Nil(ht, res2)
				checkDkgVectorError(
					ht, testCase.ExpectedError, err,
				)
			})
		}
	}
}

type dkgParticipantInvestigateExpectedOutput struct {
	DKGOutput    dkgOutputVector `json:"dkgOutput"`
	RecoveryData string          `json:"recoveryData"`
}

type dkgParticipantInvestigateTestCase struct {
	TCID          int                     `json:"tcId"`
	CInvMsg       string                  `json:"cinvMsg"`
	CMsg1Index    int                     `json:"cmsg1Index"`
	ExpectedError *dkgExpectedErrorVector `json:"expectedError"`
}

type dkgParticipantInvestigateTestGroup struct {
	Params         dkgParamsVector                     `json:"params"`
	HostSecKey     string                              `json:"hostseckey"`
	Random         string                              `json:"random"`
	AuxRand        string                              `json:"auxRand"`
	Pmsg1          string                              `json:"pmsg1"`
	Cmsg1Pool      []string                            `json:"cmsg1Pool"`
	ErrorTestCases []dkgParticipantInvestigateTestCase `json:"errorTestCases"`
}

func TestVectorsDKGParticipantInvestigate(ht *testing.T) {
	testVectorPath := path.Join(
		dkgTestVectorBaseDir, dkgParticipantInvestigateVectorsFileName,
	)
	testVectorBytes, err := os.ReadFile(testVectorPath)
	require.NoError(ht, err)

	var tests struct {
		TestGroups []dkgParticipantInvestigateTestGroup `json:"testGroups"`
	}
	require.NoError(ht, json.Unmarshal(testVectorBytes, &tests))

	runTest := func(tg dkgParticipantInvestigateTestGroup,
		testCase dkgParticipantInvestigateTestCase) error {

		params := mustReadParamsVector(tg.Params)

		hostSecKeyBytes := parseHex(tg.HostSecKey)
		hostSecKey, _ := btcec.PrivKeyFromBytes(hostSecKeyBytes)

		var random, auxRand [32]byte
		copy(random[:], parseHex(tg.Random))
		copy(auxRand[:], parseHex(tg.AuxRand))

		pstate1, pmsg1, err := ParticipantStep1(
			hostSecKey, params, &random,
		)
		if err != nil {
			return err
		}
		if !bytes.Equal(parseHex(tg.Pmsg1), pmsg1.Bytes()) {
			return errors.New("participant msg1 not equal to " +
				"expected")
		}

		cmsg1, err := ParseCoordinatorMsg1(
			parseHex(tg.Cmsg1Pool[testCase.CMsg1Index]),
			params.T, len(params.HostPubKeys),
		)
		if err != nil {
			return err
		}

		_, _, err = ParticipantStep2(
			hostSecKey, pstate1, cmsg1, &auxRand,
		)
		if err == nil {
			return errors.New("expected error, didn't get one")
		}

		invData, ok := err.(ErrUnknownFaultyParticipantOrCoordinator)
		if !ok {
			return errors.New("expected investigation data but " +
				"didn't get any")
		}

		cInvMsg, err := ParseCoordinatorInvestigationMsg(
			parseHex(testCase.CInvMsg),
			len(params.HostPubKeys),
		)
		if err != nil {
			return err
		}

		return ParticipantInvestigate(invData.InvData, cInvMsg)
	}

	for _, tg := range tests.TestGroups {
		for _, testCase := range tg.ErrorTestCases {
			tcid := fmt.Sprintf("error/%d", testCase.TCID)
			ht.Run(tcid, func(ht *testing.T) {
				err := runTest(tg, testCase)
				checkDkgVectorError(
					ht, testCase.ExpectedError, err,
				)
			})
		}
	}
}

type dkgCoordinatorStep1TestCase struct {
	TCID          int                     `json:"tcId"`
	Pmsg1Indices  []int                   `json:"pmsg1Indices"`
	Params        dkgParamsVector         `json:"params"`
	ExpectedCmsg1 *string                 `json:"expectedCmsg1"`
	ExpectedError *dkgExpectedErrorVector `json:"expectedError"`
}

type dkgCoordinatorStep1TestGroup struct {
	Pmsg1Pool      []string                      `json:"pmsg1Pool"`
	ValidTestCases []dkgCoordinatorStep1TestCase `json:"validTestCases"`
	ErrorTestCases []dkgCoordinatorStep1TestCase `json:"errorTestCases"`
}

func TestVectorsDKGCoordinatorStep1(ht *testing.T) {
	testVectorPath := path.Join(
		dkgTestVectorBaseDir, dkgCoordinatorStep1VectorsFileName,
	)
	testVectorBytes, err := os.ReadFile(testVectorPath)
	require.NoError(ht, err)

	var tests struct {
		TestGroups []dkgCoordinatorStep1TestGroup `json:"testGroups"`
	}
	require.NoError(ht, json.Unmarshal(testVectorBytes, &tests))

	runTest := func(tg dkgCoordinatorStep1TestGroup,
		testCase dkgCoordinatorStep1TestCase) (*CoordinatorMsg1,
		error) {

		params, err := readParamsVector(testCase.Params)
		if err != nil {
			return nil, err
		}

		pmsg1s := make(
			[]*ParticipantMsg1, 0, len(testCase.Pmsg1Indices),
		)

		for i, idx := range testCase.Pmsg1Indices {
			pmsg1, err := ParseParticipantMsg1(
				parseHex(tg.Pmsg1Pool[idx]),
				testCase.Params.T,
				len(testCase.Params.HostPubKeys),
			)
			if err != nil {
				if _, ok := err.(threshold.ErrMsgParse); ok {
					return nil, threshold.ErrFaultyParticipant{
						err.Error(), i,
					}
				}

				return nil, err
			}
			pmsg1s = append(pmsg1s, pmsg1)
		}

		_, cmsg1, err := CoordinatorStep1(pmsg1s, params)
		return cmsg1, err
	}

	for _, tg := range tests.TestGroups {
		for _, testCase := range tg.ValidTestCases {
			tcid := fmt.Sprintf("valid/%d", testCase.TCID)
			ht.Run(tcid, func(ht *testing.T) {
				cmsg1, err := runTest(tg, testCase)
				require.NoError(ht, err)
				expectedCmsg1Bytes := parseHex(
					*testCase.ExpectedCmsg1,
				)
				require.Equal(
					ht, expectedCmsg1Bytes, cmsg1.Bytes(),
				)

				expectedCmsg1, err := ParseCoordinatorMsg1(
					expectedCmsg1Bytes, testCase.Params.T,
					len(testCase.Params.HostPubKeys),
				)
				require.NoError(ht, err)
				require.EqualValues(ht, expectedCmsg1, cmsg1)
			})
		}

		for _, testCase := range tg.ErrorTestCases {
			tcid := fmt.Sprintf("error/%d", testCase.TCID)
			ht.Run(tcid, func(ht *testing.T) {
				res, err := runTest(tg, testCase)
				require.Nil(ht, res)
				checkDkgVectorError(
					ht, testCase.ExpectedError, err,
				)
			})
		}
	}
}

type dkgCoordinatorExpectedOutput struct {
	Cmsg2        string          `json:"cmsg2"`
	DKGOutput    dkgOutputVector `json:"dkgOutput"`
	RecoveryData string          `json:"recoveryData"`
}

type dkgCoordinatorFinalizeTestCase struct {
	TCID           int                           `json:"tcId"`
	Pmsg2Indices   []int                         `json:"pmsg2Indices"`
	ExpectedOutput *dkgCoordinatorExpectedOutput `json:"expectedOutput"`
	ExpectedError  *dkgExpectedErrorVector       `json:"expectedError"`
}

type dkgCoordinatorFinalizeTestGroup struct {
	Params         dkgParamsVector                  `json:"params"`
	Pmsgs1         []string                         `json:"pmsgs1"`
	Cmsg1          string                           `json:"cmsg1"`
	Pmsg2Pool      []string                         `json:"pmsg2Pool"`
	ValidTestCases []dkgCoordinatorFinalizeTestCase `json:"validTestCases"`
	ErrorTestCases []dkgCoordinatorFinalizeTestCase `json:"errorTestCases"`
}

func TestVectorsDKGCoordinatorFinalize(ht *testing.T) {
	testVectorPath := path.Join(
		dkgTestVectorBaseDir, dkgCoordinatorFinalizeVectorsFileName,
	)
	testVectorBytes, err := os.ReadFile(testVectorPath)
	require.NoError(ht, err)

	var tests struct {
		TestGroups []dkgCoordinatorFinalizeTestGroup `json:"testGroups"`
	}
	require.NoError(ht, json.Unmarshal(testVectorBytes, &tests))

	runTest := func(tg dkgCoordinatorFinalizeTestGroup,
		testCase dkgCoordinatorFinalizeTestCase) (*CoordinatorMsg2,
		*threshold.DKGOutput, *RecoveryData, error) {

		params := mustReadParamsVector(tg.Params)

		pmsg1s := make(
			[]*ParticipantMsg1, 0, len(tg.Pmsgs1),
		)

		for _, pmsg1Hex := range tg.Pmsgs1 {
			pmsg1, err := ParseParticipantMsg1(
				parseHex(pmsg1Hex), tg.Params.T,
				len(tg.Params.HostPubKeys),
			)
			if err != nil {
				return nil, nil, nil, err
			}
			pmsg1s = append(pmsg1s, pmsg1)
		}

		cState, cMsg1, err := CoordinatorStep1(pmsg1s, params)
		if err != nil {
			return nil, nil, nil, err
		}
		if !bytes.Equal(parseHex(tg.Cmsg1), cMsg1.Bytes()) {
			return nil, nil, nil, errors.New("coordinator msg1 " +
				"not equal to expected")
		}

		pmsg2s := make(
			[]*ParticipantMsg2, 0, len(testCase.Pmsg2Indices),
		)

		for _, idx := range testCase.Pmsg2Indices {
			pmsg2, err := ParseParticipantMsg2(
				parseHex(tg.Pmsg2Pool[idx]),
			)
			if err != nil {
				return nil, nil, nil, err
			}

			pmsg2s = append(pmsg2s, pmsg2)
		}

		return CoordinatorFinalize(cState, pmsg2s)
	}

	for _, tg := range tests.TestGroups {
		for _, testCase := range tg.ValidTestCases {
			tcid := fmt.Sprintf("valid/%d", testCase.TCID)
			ht.Run(tcid, func(ht *testing.T) {
				cMsg2, dkgOutput, recData, err := runTest(
					tg, testCase,
				)
				require.NoError(ht, err)
				require.Equal(
					ht, testCase.ExpectedOutput.Cmsg2,
					fmt.Sprintf("%X", cMsg2.Bytes()),
				)
				require.Equal(ht, parseHex(
					testCase.ExpectedOutput.RecoveryData,
				), []byte(*recData))
				require.Equal(ht, parseDkgOutputVector(
					ht, testCase.ExpectedOutput.DKGOutput,
				), dkgOutput)
			})
		}

		for _, testCase := range tg.ErrorTestCases {
			tcid := fmt.Sprintf("error/%d", testCase.TCID)
			ht.Run(tcid, func(ht *testing.T) {
				res1, res2, res3, err := runTest(tg, testCase)
				require.Nil(ht, res1)
				require.Nil(ht, res2)
				require.Nil(ht, res3)
				checkDkgVectorError(
					ht, testCase.ExpectedError, err,
				)
			})
		}
	}
}

type dkgCoordinatorInvestigateTestCase struct {
	TCID             int      `json:"tcId"`
	ExpectedCinvMsgs []string `json:"expectedCinvMsgs"`
}

type dkgCoordinatorInvestigateTestGroup struct {
	Params         dkgParamsVector                     `json:"params"`
	Pmsgs1         []string                            `json:"pmsgs1"`
	ValidTestCases []dkgCoordinatorInvestigateTestCase `json:"validTestCases"`
}

func TestVectorsDKGCoordinatorInvestigate(ht *testing.T) {
	testVectorPath := path.Join(
		dkgTestVectorBaseDir, dkgCoordinatorInvestigateVectorsFileName,
	)
	testVectorBytes, err := os.ReadFile(testVectorPath)
	require.NoError(ht, err)

	var tests struct {
		TestGroups []dkgCoordinatorInvestigateTestGroup `json:"testGroups"`
	}
	require.NoError(ht, json.Unmarshal(testVectorBytes, &tests))

	runTest := func(tg dkgCoordinatorInvestigateTestGroup,
		testCase dkgCoordinatorInvestigateTestCase) []string {

		params := mustReadParamsVector(tg.Params)

		pmsg1s := make(
			[]*ParticipantMsg1, 0, len(tg.Pmsgs1),
		)

		for _, pmsg1Hex := range tg.Pmsgs1 {
			pmsg1, err := ParseParticipantMsg1(
				parseHex(pmsg1Hex), params.T,
				len(params.HostPubKeys),
			)
			if err != nil {
				panic(err)
			}
			pmsg1s = append(pmsg1s, pmsg1)
		}

		cInvMsgs := CoordinatorInvestigate(pmsg1s)

		cInvMsg1sHex := make([]string, 0, len(cInvMsgs))
		for _, msg := range cInvMsgs {
			cInvMsg1sHex = append(
				cInvMsg1sHex, fmt.Sprintf("%X", msg.Bytes()),
			)
		}

		return cInvMsg1sHex
	}

	for _, tg := range tests.TestGroups {
		for _, testCase := range tg.ValidTestCases {
			tcid := fmt.Sprintf("valid/%d", testCase.TCID)
			ht.Run(tcid, func(ht *testing.T) {
				cInvMsgs := runTest(tg, testCase)
				require.NoError(ht, err)
				require.Equal(
					ht, testCase.ExpectedCinvMsgs,
					cInvMsgs,
				)
			})
		}
	}
}

type dkgRecoverExpectedOutput struct {
	DKGOutput dkgOutputVector `json:"dkgOutput"`
	Params    dkgParamsVector `json:"params"`
}

type dkgRecoverTestCase struct {
	TCID           int                       `json:"tcId"`
	HostSecKey     *string                   `json:"hostseckey"`
	RecoveryData   string                    `json:"recoveryData"`
	ExpectedOutput *dkgRecoverExpectedOutput `json:"expectedOutput"`
	ExpectedError  *dkgExpectedErrorVector   `json:"expectedError"`
}

type dkgRecoverTestCases struct {
	ValidTestCases []dkgRecoverTestCase `json:"validTestCases"`
	ErrorTestCases []dkgRecoverTestCase `json:"errorTestCases"`
}

func TestVectorsDKGRecover(ht *testing.T) {
	testVectorPath := path.Join(
		dkgTestVectorBaseDir, dkgRecoverVectorsFileName,
	)
	testVectorBytes, err := os.ReadFile(testVectorPath)
	require.NoError(ht, err)

	var testCases dkgRecoverTestCases
	require.NoError(ht, json.Unmarshal(testVectorBytes, &testCases))

	runTest := func(testCase dkgRecoverTestCase) (*threshold.DKGOutput,
		*SessionParams, error) {

		var (
			hostSecKey *btcec.PrivateKey
			err        error
		)
		if testCase.HostSecKey != nil {
			hostSecKey, _, err = hostPubKeyGen(
				parseHex(*testCase.HostSecKey),
			)
			if err != nil {
				return nil, nil, err
			}
		}

		recData := RecoveryData(
			parseHex(testCase.RecoveryData),
		)

		return Recover(hostSecKey, &recData)
	}

	for _, testCase := range testCases.ValidTestCases {
		tcid := fmt.Sprintf("valid/%d", testCase.TCID)
		ht.Run(tcid, func(ht *testing.T) {
			dkgOutput, params, err := runTest(testCase)
			require.NoError(ht, err)
			require.Equal(
				ht, parseDkgOutputVector(
					ht, testCase.ExpectedOutput.DKGOutput,
				), dkgOutput,
			)

			expectedParams := mustReadParamsVector(
				testCase.ExpectedOutput.Params,
			)
			require.Equal(ht, expectedParams, params)
		})
	}

	for _, testCase := range testCases.ErrorTestCases {
		tcid := fmt.Sprintf("error/%d", testCase.TCID)
		ht.Run(tcid, func(ht *testing.T) {
			res1, res2, err := runTest(testCase)
			require.Nil(ht, res1)
			require.Nil(ht, res2)
			checkDkgVectorError(ht, testCase.ExpectedError, err)
		})
	}
}

func parseHex(msg string) []byte {
	b, err := hex.DecodeString(msg)
	if err != nil {
		panic(err)
	}

	return b
}

func readPubKeys(hexPubkeys []string) ([]*btcec.PublicKey, error) {
	pubKeys := make([]*btcec.PublicKey, 0, len(hexPubkeys))

	for i, hexKey := range hexPubkeys {
		keyBytes, err := hex.DecodeString(hexKey)
		if err != nil {
			return nil, err
		}

		key, err := btcec.ParsePubKey(keyBytes)
		if err != nil {
			return nil, errInvalidHostPubKey(i)
		}

		pubKeys = append(pubKeys, key)
	}

	return pubKeys, nil
}

func readParamsVector(paramsVector dkgParamsVector) (*SessionParams, error) {
	keys, err := readPubKeys(paramsVector.HostPubKeys)
	if err != nil {
		return nil, err
	}

	params := &SessionParams{
		HostPubKeys: keys,
		T:           paramsVector.T,
	}

	err = ParamsValidate(params)
	if err != nil {
		return nil, err
	}

	return params, nil
}

func mustReadParamsVector(paramsVector dkgParamsVector) *SessionParams {
	params, err := readParamsVector(paramsVector)
	if err != nil {
		panic(err)
	}

	return params
}

func parseDkgOutputVector(ht *testing.T, in dkgOutputVector) *threshold.DKGOutput {
	var (
		out threshold.DKGOutput
		err error
	)

	if in.SecShare != nil {
		out.SecShare, _ = btcec.PrivKeyFromBytes(
			parseHex(*in.SecShare),
		)
	}

	out.ThresholdPubKey, err = btcec.ParsePubKey(
		parseHex(in.ThresholdPubKey),
	)
	require.NoError(ht, err)

	out.PubShares, err = readPubKeys(in.PubShares)
	require.NoError(ht, err)

	return &out
}
