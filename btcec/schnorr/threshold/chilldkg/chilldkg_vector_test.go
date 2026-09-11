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
	"github.com/btcsuite/btcd/btcec/v2/schnorr/threshold/util"
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
		require.ErrorIs(ht, err, util.ErrFaultyCoordinator(eMsg))

	case "RecoveryDataError":
		require.ErrorIs(ht, err, ErrRecoveryData(eMsg))

	case "FaultyParticipantOrCoordinatorError":
		require.ErrorIs(ht, err, util.ErrFaultyParticipantOrCoordinator{
			eMsg, *expected.Participant,
		})

	case "FaultyParticipantError":
		require.ErrorIs(ht, err, util.ErrFaultyParticipant{
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

func TestHostPubKeyGenVectors(ht *testing.T) {
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
			parseHex(ht, testCase.HostSecKey),
		)

		return pubKey, err
	}

	for _, testCase := range testCases.ValidTestCases {
		tcid := fmt.Sprintf("valid/%d", testCase.TCID)
		ht.Run(tcid, func(ht *testing.T) {
			res, err := runTest(testCase)
			require.NoError(ht, err)
			require.Equal(
				ht, parseHex(ht, *testCase.ExpectedHostPubKey),
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

func TestParamsHashVectors(ht *testing.T) {
	testVectorPath := path.Join(
		dkgTestVectorBaseDir, dkgParamsHashVectorsFileName,
	)
	testVectorBytes, err := os.ReadFile(testVectorPath)
	require.NoError(ht, err)

	var testCases dkgParamsHashTestCases
	require.NoError(ht, json.Unmarshal(testVectorBytes, &testCases))

	runTest := func(testCase dkgParamsHashTestCase) (*chainhash.Hash,
		error) {

		keys, err := readPubKeys(testCase.Params.HostPubKeys)
		if err != nil {
			return nil, err
		}

		params := &SessionParams{
			HostPubKeys: keys,
			T:           testCase.Params.T,
		}
		return ParamsHash(params)
	}

	for _, testCase := range testCases.ValidTestCases {
		tcid := fmt.Sprintf("valid/%d", testCase.TCID)
		ht.Run(tcid, func(ht *testing.T) {
			res, err := runTest(testCase)
			require.NoError(ht, err)
			require.Equal(
				ht, parseHex(ht, *testCase.ExpectedParamsHash),
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

func TestParticipantStep1Vectors(ht *testing.T) {
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

	runTest := func(ht *testing.T, testCase dkgParticipantStep1TestCase) (
		*ParticipantMsg1, error) {

		keys, err := readPubKeys(testCase.Params.HostPubKeys)
		if err != nil {
			return nil, err
		}

		params := &SessionParams{
			HostPubKeys: keys,
			T:           testCase.Params.T,
		}
		_, err = ParamsHash(params)
		if err != nil {
			return nil, err
		}

		hostSecKey, _, err := hostPubKeyGen(
			parseHex(ht, testCase.HostSecKey),
		)
		if err != nil {
			return nil, err
		}

		randBytes := parseHex(ht, testCase.Random)
		if len(randBytes) != 32 || bytes.Equal(
			randBytes, allZeroes[:],
		) {

			return nil, errRandomness
		}

		var random [32]byte
		copy(random[:], randBytes)

		_, pmsg1, err := ParticipantStep1(hostSecKey, params, random)
		return pmsg1, err
	}

	for _, tg := range tests.TestGroups {
		for _, testCase := range tg.ValidTestCases {
			tcid := fmt.Sprintf("valid/%d", testCase.TCID)
			ht.Run(tcid, func(ht *testing.T) {
				res, err := runTest(ht, testCase)
				require.NoError(ht, err)

				require.Equal(
					ht, parseHex(
						ht, *testCase.ExpectedPmsg1,
					), res.Bytes(),
				)
			})
		}

		for _, testCase := range tg.ErrorTestCases {
			tcid := fmt.Sprintf("error/%d", testCase.TCID)
			ht.Run(tcid, func(ht *testing.T) {
				res, err := runTest(ht, testCase)
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

func TestParticipantStep2Vectors(ht *testing.T) {
	testVectorPath := path.Join(
		dkgTestVectorBaseDir, dkgParticipantStep2VectorsFileName,
	)
	testVectorBytes, err := os.ReadFile(testVectorPath)
	require.NoError(ht, err)

	var tests struct {
		TestGroups []dkgParticipantStep2TestGroup `json:"testGroups"`
	}
	require.NoError(ht, json.Unmarshal(testVectorBytes, &tests))

	runTest := func(ht *testing.T, tg dkgParticipantStep2TestGroup,
		testCase dkgParticipantStep2TestCase) (*ParticipantMsg2,
		error) {

		keys, err := readPubKeys(tg.Params.HostPubKeys)
		require.NoError(ht, err)

		params := &SessionParams{
			HostPubKeys: keys,
			T:           tg.Params.T,
		}
		_, err = ParamsHash(params)
		require.NoError(ht, err)

		hostSecKey, _, err := hostPubKeyGen(parseHex(ht, tg.HostSecKey))
		require.NoError(ht, err)

		var random, auxRand [32]byte
		copy(random[:], parseHex(ht, tg.Random))
		copy(auxRand[:], parseHex(ht, tg.AuxRand))

		if testCase.AuxRand != nil {
			auxRandBytes := parseHex(ht, *testCase.AuxRand)
			if len(auxRandBytes) != 32 {
				// ValueError with no message
				return nil, fmt.Errorf("")
			}
			copy(auxRand[:], auxRandBytes)
		}

		pstate1, pmsg1, err := ParticipantStep1(
			hostSecKey, params, random,
		)
		require.NoError(ht, err)
		require.Equal(ht, parseHex(ht, tg.Pmsg1), pmsg1.Bytes())

		cmsg1, err := ParseCoordinatorMsg1(
			parseHex(ht, testCase.Cmsg1), params.T,
			len(params.HostPubKeys),
		)
		if err != nil {
			if fpcErr, ok := err.(util.
				ErrFaultyParticipantOrCoordinator); ok &&
				fpcErr.Participant == pstate1.Idx {

				return nil, util.ErrFaultyCoordinator(
					"Coordinator replied with wrong " +
						"pubnonce")
			}

			return nil, err
		}

		if testCase.HostSecKey != nil {
			hostSecKey, _, err = hostPubKeyGen(
				parseHex(ht, *testCase.HostSecKey),
			)
			require.NoError(ht, err)
		}

		_, pmsg2, err := ParticipantStep2(
			hostSecKey, pstate1, cmsg1, auxRand,
		)
		return pmsg2, err
	}

	for _, tg := range tests.TestGroups {
		for _, testCase := range tg.ValidTestCases {
			tcid := fmt.Sprintf("valid/%d", testCase.TCID)
			ht.Run(tcid, func(ht *testing.T) {
				res, err := runTest(ht, tg, testCase)
				require.NoError(ht, err)
				require.Equal(
					ht, parseHex(
						ht, *testCase.ExpectedPmsg2,
					), res.Bytes(),
				)
			})
		}

		for _, testCase := range tg.ErrorTestCases {
			tcid := fmt.Sprintf("error/%d", testCase.TCID)
			ht.Run(tcid, func(ht *testing.T) {
				res, err := runTest(ht, tg, testCase)
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

func TestParticipantFinalizeVectors(ht *testing.T) {
	testVectorPath := path.Join(
		dkgTestVectorBaseDir, dkgParticipantFinalizeVectorsFileName,
	)
	testVectorBytes, err := os.ReadFile(testVectorPath)
	require.NoError(ht, err)

	var tests struct {
		TestGroups []dkgParticipantFinalizeTestGroup `json:"testGroups"`
	}
	require.NoError(ht, json.Unmarshal(testVectorBytes, &tests))

	runTest := func(ht *testing.T, tg dkgParticipantFinalizeTestGroup,
		testCase dkgParticipantFinalizeTestCase) (*util.DKGOutput,
		*RecoveryData, error) {

		keys, err := readPubKeys(tg.Params.HostPubKeys)
		require.NoError(ht, err)

		params := &SessionParams{
			HostPubKeys: keys,
			T:           tg.Params.T,
		}
		_, err = ParamsHash(params)
		require.NoError(ht, err)

		hostSecKeyBytes := parseHex(ht, tg.HostSecKey)
		hostSecKey, _ := btcec.PrivKeyFromBytes(hostSecKeyBytes)

		var random, auxRand [32]byte
		copy(random[:], parseHex(ht, tg.Random))
		copy(auxRand[:], parseHex(ht, tg.AuxRand))

		pstate1, pmsg1, err := ParticipantStep1(
			hostSecKey, params, random,
		)
		require.NoError(ht, err)
		require.Equal(ht, pmsg1.Bytes(), parseHex(ht, tg.Pmsg1))

		cmsg1, err := ParseCoordinatorMsg1(
			parseHex(ht, tg.Cmsg1), params.T,
			len(params.HostPubKeys),
		)
		require.NoError(ht, err)

		pstate2, pmsg2, err := ParticipantStep2(
			hostSecKey, pstate1, cmsg1, auxRand,
		)
		require.NoError(ht, err)
		require.Equal(
			ht, parseHex(ht, tg.Pmsg2), pmsg2.Bytes(),
		)

		cmsg2, err := ParseCoordinatorMsg2(
			parseHex(ht, testCase.Cmsg2),
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
				dkgOutput, recData, err := runTest(
					ht, tg, testCase,
				)
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
				res1, res2, err := runTest(
					ht, tg, testCase,
				)
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

func TestParticipantInvestigateVectors(ht *testing.T) {
	testVectorPath := path.Join(
		dkgTestVectorBaseDir, dkgParticipantInvestigateVectorsFileName,
	)
	testVectorBytes, err := os.ReadFile(testVectorPath)
	require.NoError(ht, err)

	var tests struct {
		TestGroups []dkgParticipantInvestigateTestGroup `json:"testGroups"`
	}
	require.NoError(ht, json.Unmarshal(testVectorBytes, &tests))

	runTest := func(ht *testing.T, tg dkgParticipantInvestigateTestGroup,
		testCase dkgParticipantInvestigateTestCase) error {

		keys, err := readPubKeys(tg.Params.HostPubKeys)
		require.NoError(ht, err)

		params := &SessionParams{
			HostPubKeys: keys,
			T:           tg.Params.T,
		}
		_, err = ParamsHash(params)
		require.NoError(ht, err)

		hostSecKeyBytes := parseHex(ht, tg.HostSecKey)
		hostSecKey, _ := btcec.PrivKeyFromBytes(hostSecKeyBytes)

		var random, auxRand [32]byte
		copy(random[:], parseHex(ht, tg.Random))
		copy(auxRand[:], parseHex(ht, tg.AuxRand))

		pstate1, pmsg1, err := ParticipantStep1(
			hostSecKey, params, random,
		)
		require.NoError(ht, err)
		require.Equal(ht, pmsg1.Bytes(), parseHex(ht, tg.Pmsg1))

		cmsg1, err := ParseCoordinatorMsg1(
			parseHex(ht, tg.Cmsg1Pool[testCase.CMsg1Index]),
			params.T, len(params.HostPubKeys),
		)
		require.NoError(ht, err)

		_, _, err = ParticipantStep2(
			hostSecKey, pstate1, cmsg1, auxRand,
		)
		require.Error(ht, err)

		invData, ok := err.(ErrUnknownFaultyParticipantOrCoordinator)
		require.True(ht, ok)

		cInvMsg, err := ParseCoordinatorInvestigationMsg(
			parseHex(ht, testCase.CInvMsg),
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
				err := runTest(ht, tg, testCase)
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

func TestCoordinatorStep1Vectors(ht *testing.T) {
	testVectorPath := path.Join(
		dkgTestVectorBaseDir, dkgCoordinatorStep1VectorsFileName,
	)
	testVectorBytes, err := os.ReadFile(testVectorPath)
	require.NoError(ht, err)

	var tests struct {
		TestGroups []dkgCoordinatorStep1TestGroup `json:"testGroups"`
	}
	require.NoError(ht, json.Unmarshal(testVectorBytes, &tests))

	runTest := func(ht *testing.T, tg dkgCoordinatorStep1TestGroup,
		testCase dkgCoordinatorStep1TestCase) (*CoordinatorMsg1,
		error) {

		keys, err := readPubKeys(testCase.Params.HostPubKeys)
		if err != nil {
			return nil, err
		}

		params := &SessionParams{
			HostPubKeys: keys,
			T:           testCase.Params.T,
		}
		_, err = ParamsHash(params)
		if err != nil {
			return nil, err
		}

		pmsg1s := make(
			[]*ParticipantMsg1, 0, len(testCase.Pmsg1Indices),
		)

		for i, idx := range testCase.Pmsg1Indices {
			pmsg1, err := ParseParticipantMsg1(
				parseHex(ht, tg.Pmsg1Pool[idx]),
				testCase.Params.T,
				len(testCase.Params.HostPubKeys),
			)
			if err != nil {
				if _, ok := err.(util.ErrMsgParse); ok {
					return nil, util.ErrFaultyParticipant{
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
				cmsg1, err := runTest(ht, tg, testCase)
				require.NoError(ht, err)
				expectedCmsg1Bytes := parseHex(
					ht, *testCase.ExpectedCmsg1,
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
				res, err := runTest(ht, tg, testCase)
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

func TestCoordinatorFinalizeVectors(ht *testing.T) {
	testVectorPath := path.Join(
		dkgTestVectorBaseDir, dkgCoordinatorFinalizeVectorsFileName,
	)
	testVectorBytes, err := os.ReadFile(testVectorPath)
	require.NoError(ht, err)

	var tests struct {
		TestGroups []dkgCoordinatorFinalizeTestGroup `json:"testGroups"`
	}
	require.NoError(ht, json.Unmarshal(testVectorBytes, &tests))

	runTest := func(ht *testing.T, tg dkgCoordinatorFinalizeTestGroup,
		testCase dkgCoordinatorFinalizeTestCase) (*CoordinatorMsg2,
		*util.DKGOutput, *RecoveryData, error) {

		keys, err := readPubKeys(tg.Params.HostPubKeys)
		require.NoError(ht, err)

		params := &SessionParams{
			HostPubKeys: keys,
			T:           tg.Params.T,
		}
		_, err = ParamsHash(params)
		require.NoError(ht, err)

		pmsg1s := make(
			[]*ParticipantMsg1, 0, len(tg.Pmsgs1),
		)

		for _, pmsg1Hex := range tg.Pmsgs1 {
			pmsg1, err := ParseParticipantMsg1(
				parseHex(ht, pmsg1Hex), tg.Params.T,
				len(tg.Params.HostPubKeys),
			)
			require.NoError(ht, err)
			pmsg1s = append(pmsg1s, pmsg1)
		}

		cState, cMsg1, err := CoordinatorStep1(pmsg1s, params)
		require.NoError(ht, err)
		require.Equal(ht, parseHex(ht, tg.Cmsg1), cMsg1.Bytes())

		pmsg2s := make(
			[]*ParticipantMsg2, 0, len(testCase.Pmsg2Indices),
		)

		for _, idx := range testCase.Pmsg2Indices {
			pmsg2, err := ParseParticipantMsg2(
				parseHex(ht, tg.Pmsg2Pool[idx]),
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
					ht, tg, testCase,
				)
				require.NoError(ht, err)
				require.Equal(
					ht, testCase.ExpectedOutput.Cmsg2,
					fmt.Sprintf("%X", cMsg2.Bytes()),
				)
				require.Equal(ht, parseHex(
					ht,
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
				res1, res2, res3, err := runTest(
					ht, tg, testCase,
				)
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

func TestCoordinatorInvestigateVectors(ht *testing.T) {
	testVectorPath := path.Join(
		dkgTestVectorBaseDir, dkgCoordinatorInvestigateVectorsFileName,
	)
	testVectorBytes, err := os.ReadFile(testVectorPath)
	require.NoError(ht, err)

	var tests struct {
		TestGroups []dkgCoordinatorInvestigateTestGroup `json:"testGroups"`
	}
	require.NoError(ht, json.Unmarshal(testVectorBytes, &tests))

	runTest := func(ht *testing.T, tg dkgCoordinatorInvestigateTestGroup,
		testCase dkgCoordinatorInvestigateTestCase) []string {

		keys, err := readPubKeys(tg.Params.HostPubKeys)
		require.NoError(ht, err)

		params := &SessionParams{
			HostPubKeys: keys,
			T:           tg.Params.T,
		}
		_, err = ParamsHash(params)
		require.NoError(ht, err)

		pmsg1s := make(
			[]*ParticipantMsg1, 0, len(tg.Pmsgs1),
		)

		for _, pmsg1Hex := range tg.Pmsgs1 {
			pmsg1, err := ParseParticipantMsg1(
				parseHex(ht, pmsg1Hex), tg.Params.T,
				len(tg.Params.HostPubKeys),
			)
			require.NoError(ht, err)
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
				cInvMsgs := runTest(ht, tg, testCase)
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

func TestRecoverVectors(ht *testing.T) {
	testVectorPath := path.Join(
		dkgTestVectorBaseDir, dkgRecoverVectorsFileName,
	)
	testVectorBytes, err := os.ReadFile(testVectorPath)
	require.NoError(ht, err)

	var testCases dkgRecoverTestCases
	require.NoError(ht, json.Unmarshal(testVectorBytes, &testCases))

	runTest := func(testCase dkgRecoverTestCase) (*util.DKGOutput,
		*SessionParams, error) {

		var (
			hostSecKey *btcec.PrivateKey
			err        error
		)
		if testCase.HostSecKey != nil {
			hostSecKey, _, err = hostPubKeyGen(
				parseHex(ht, *testCase.HostSecKey),
			)
			if err != nil {
				return nil, nil, err
			}
		}

		recData := RecoveryData(
			parseHex(ht, testCase.RecoveryData),
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

			expectedParams := &SessionParams{
				T: testCase.ExpectedOutput.Params.T,
			}

			expectedParams.HostPubKeys, err = readPubKeys(
				testCase.ExpectedOutput.Params.HostPubKeys,
			)
			require.NoError(ht, err)
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

func parseHex(ht *testing.T, msg string) []byte {
	b, err := hex.DecodeString(msg)
	require.NoError(ht, err)

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

func parseDkgOutputVector(ht *testing.T, in dkgOutputVector) *util.DKGOutput {
	var (
		out util.DKGOutput
		err error
	)

	if in.SecShare != nil {
		out.SecShare, _ = btcec.PrivKeyFromBytes(
			parseHex(ht, *in.SecShare),
		)
	}

	out.ThresholdPubKey, err = btcec.ParsePubKey(
		parseHex(ht, in.ThresholdPubKey),
	)
	require.NoError(ht, err)

	out.PubShares, err = readPubKeys(in.PubShares)
	require.NoError(ht, err)

	return &out
}
