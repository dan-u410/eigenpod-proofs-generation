package eigenpodproofs

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"testing"

	ssz "github.com/ferranbt/fastssz"
	"github.com/stretchr/testify/assert"

	"github.com/attestantio/go-eth2-client/spec/deneb"
	"github.com/attestantio/go-eth2-client/spec/phase0"
)

var (
	b                              deneb.BeaconState
	blockHeader                    phase0.BeaconBlockHeader
	blockHeaderIndex               uint64
	block                          deneb.BeaconBlock
	validatorIndex                 phase0.ValidatorIndex
	beaconBlockHeaderToVerifyIndex uint64
	executionPayload               deneb.ExecutionPayload
	epp                            *EigenPodProofs
)

// var VALIDATOR_INDEX uint64 = 61068 //this is the index of a validator that has a partial withdrawal
var VALIDATOR_INDEX uint64 = 61336           //this is the index of a validator that has a full withdrawal.
var REPOINTED_VALIDATOR_INDEX uint64 = 61511 //this is the index of a validator that we use for the withdrawal credential proofs

// this needs to be hand crafted. If you want the root of the header at the slot x,
// then look for entry in (x)%slotsPerHistoricalRoot in the block_roots.

// var BEACON_BLOCK_HEADER_TO_VERIFY_INDEX uint64 = 656
var BEACON_BLOCK_HEADER_TO_VERIFY_INDEX uint64 = 2262

var GOERLI_CHAIN_ID uint64 = 5

func TestMain(m *testing.M) {
	// Setup
	log.Println("Setting up suite")
	setupSuite()

	// Run tests
	code := m.Run()

	// Teardown
	log.Println("Tearing down suite")
	teardownSuite()

	// Exit with test result code
	os.Exit(code)
}

func setupSuite() {
	log.Println("Setting up suite")
	stateFile := "data/deneb_goerli_slot_7413760.json"
	headerFile := "data/deneb_goerli_block_header_7426113.json"
	bodyFile := "data/deneb_goerli_block_7426113.json"

	//ParseCapellaBeaconState(stateFile)

	stateJSON, err := parseJSONFile(stateFile)
	if err != nil {
		fmt.Println("error with JSON parsing beacon state")
	}
	ParseDenebBeaconStateFromJSON(*stateJSON, &b)

	blockHeader, err = ExtractBlockHeader(headerFile)
	if err != nil {
		fmt.Println("error with block header", err)
	}

	block, err = ExtractBlock(bodyFile)
	if err != nil {
		fmt.Println("error with block body", err)
	}

	executionPayload = *block.Body.ExecutionPayload

	blockHeaderIndex = uint64(blockHeader.Slot) % slotsPerHistoricalRoot

	epp, err = NewEigenPodProofs(GOERLI_CHAIN_ID, 1000)
	if err != nil {
		fmt.Println("error in NewEigenPodProofs", err)
	}

}

func teardownSuite() {
	// Any cleanup you want to perform should go here
	fmt.Println("all done!")
}

func TestGenerateWithdrawalCredentialsProof(t *testing.T) {

	// picking up one random validator index
	validatorIndex := phase0.ValidatorIndex(REPOINTED_VALIDATOR_INDEX)

	beaconStateTopLevelRoots, err := ComputeBeaconStateTopLevelRoots(&b)
	if err != nil {
		fmt.Println("error reading beaconStateTopLevelRoots")
	}

	proof, err := epp.ProveValidatorAgainstBeaconState(&b, beaconStateTopLevelRoots, uint64(validatorIndex))
	if err != nil {
		fmt.Println(err)
	}
	leaf, err := b.Validators[validatorIndex].HashTreeRoot()
	if err != nil {
		fmt.Println("error with hash tree root")
	}

	root, err := b.HashTreeRoot()
	if err != nil {
		fmt.Println("error with hash tree root of beacon state")
	}

	index := validatorListIndex<<(validatorListMerkleSubtreeNumLayers+1) | uint64(validatorIndex)

	flag := ValidateProof(root, proof, leaf, index)
	if flag != true {
		fmt.Println("error")
	}

	assert.True(t, flag, "Proof %v failed")
}

func TestProveValidatorBalanceAgainstValidatorBalanceList(t *testing.T) {

	validatorIndex := phase0.ValidatorIndex(REPOINTED_VALIDATOR_INDEX)
	proof, _ := ProveValidatorBalanceAgainstValidatorBalanceList(b.Balances, uint64(validatorIndex))

	beaconStateTopLevelRoots, _ := ComputeBeaconStateTopLevelRoots(&b)
	root := beaconStateTopLevelRoots.BalancesRoot

	balanceRootList, err := GetBalanceRoots(b.Balances)
	if err != nil {
		fmt.Println("error", err)
	}

	balanceIndex := validatorIndex / 4

	leaf := balanceRootList[balanceIndex]

	flag := ValidateProof(*root, proof, leaf, uint64(balanceIndex))
	if flag != true {
		fmt.Println("balance proof failed")
	}
	assert.True(t, flag, "Proof %v failed")
}

func TestProveBeaconTopLevelRootAgainstBeaconState(t *testing.T) {

	// get the oracle state root for a merkle tree with top level roots as the leaves
	beaconStateTopLevelRoots, err := ComputeBeaconStateTopLevelRoots(&b)
	if err != nil {
		fmt.Println("error")
	}

	// compute the Merkle proof for the inclusion of Validators Root as a leaf
	validatorsRootProof, err := ProveBeaconTopLevelRootAgainstBeaconState(beaconStateTopLevelRoots, validatorListIndex)
	if err != nil {
		fmt.Println("error")
	}

	// getting Merkle root of the BeaconStateRoot Merkle tree from attestation's code
	beaconStateRoot, err := b.HashTreeRoot()
	if err != nil {
		fmt.Println("error")
	}

	// validation of the proof
	// get the leaf denoting the validatorsRoot in the BeaconStateRoot Merkle tree
	leaf := beaconStateTopLevelRoots.ValidatorsRoot
	flag := ValidateProof(beaconStateRoot, validatorsRootProof, *leaf, validatorListIndex)
	if flag != true {
		fmt.Println("error")
	}
	// fmt.Println("flag", flag)

	assert.True(t, flag, "Proof %v failed\n")
}

func TestGetHistoricalSummariesBlockRootsProofProof(t *testing.T) {

	//curl -H "Accept: application/json" https://data.spiceai.io/goerli/beacon/eth/v2/debug/beacon/states/7431952 -o deneb_goerli_slot_7431952.json --header 'X-API-Key: 343035|8b6ddd9b31f54c07b3fc18282b30f61c'
	currentBeaconStateJSON, err := parseJSONFile("data/deneb_goerli_slot_7431952.json")

	if err != nil {
		fmt.Println("error parsing currentBeaconStateJSON")
	}

	//this is not the beacon state of the slot containing the old withdrawal we want to proof but rather
	// its the state that was merklized to create a historical summary containing the slot that has that withdrawal, ie, 7421952 mod 8192 = 0
	oldBeaconStateJSON, err := parseJSONFile("data/deneb_goerli_slot_7421952.json")
	if err != nil {
		fmt.Println("error parsing oldBeaconStateJSON")
	}

	var blockHeader phase0.BeaconBlockHeader
	//blockHeader, err = ExtractBlockHeader("data/goerli_block_header_6397852.json")
	blockHeader, err = ExtractBlockHeader("data/deneb_goerli_block_header_7421951.json")

	if err != nil {
		fmt.Println("blockHeader.UnmarshalJSON error", err)
	}

	var currentBeaconState deneb.BeaconState
	var oldBeaconState deneb.BeaconState

	ParseDenebBeaconStateFromJSON(*currentBeaconStateJSON, &currentBeaconState)
	ParseDenebBeaconStateFromJSON(*oldBeaconStateJSON, &oldBeaconState)
	fmt.Println("currentBeacon state historical summary lentgh is", len(currentBeaconState.HistoricalSummaries))

	currentBeaconStateTopLevelRoots, _ := ComputeBeaconStateTopLevelRoots(&currentBeaconState)
	//oldBeaconStateTopLevelRoots, _ := ComputeBeaconStateTopLevelRoots(&oldBeaconState)

	if err != nil {
		fmt.Println("error")
	}

	historicalSummaryIndex := uint64(271)
	beaconBlockHeaderToVerifyIndex = 8191 //(7421951 mod 8192)
	beaconBlockHeaderToVerify, err := blockHeader.HashTreeRoot()
	if err != nil {
		fmt.Println("error", err)
	}

	// fmt.Println("THESE SHOULD BE", hex.EncodeToString(beaconBlockHeaderToVerify[:]))
	// fmt.Println("THE SAME", hex.EncodeToString(beaconBlockHeaderToVerify[:]))
	// fmt.Println("THESE SHOULD BE", hex.EncodeToString(oldBeaconStateTopLevelRoots.BlockRootsRoot[:]))
	// fmt.Println("THE SAME", hex.EncodeToString(currentBeaconState.HistoricalSummaries[146].BlockSummaryRoot[:]))

	oldBlockRoots := oldBeaconState.BlockRoots

	historicalSummaryBlockHeaderProof, err := ProveBlockRootAgainstBeaconStateViaHistoricalSummaries(
		currentBeaconStateTopLevelRoots,
		currentBeaconState.HistoricalSummaries,
		oldBlockRoots,
		historicalSummaryIndex,
		beaconBlockHeaderToVerifyIndex,
	)

	if err != nil {
		fmt.Println("error")
	}

	currentBeaconStateRoot, _ := currentBeaconState.HashTreeRoot()

	historicalBlockHeaderIndex := historicalSummaryListIndex<<((historicalSummaryListMerkleSubtreeNumLayers+1)+1+(blockRootsMerkleSubtreeNumLayers)) |
		historicalSummaryIndex<<(1+blockRootsMerkleSubtreeNumLayers) |
		blockSummaryRootIndex<<(blockRootsMerkleSubtreeNumLayers) | beaconBlockHeaderToVerifyIndex

	flag := ValidateProof(currentBeaconStateRoot, historicalSummaryBlockHeaderProof, beaconBlockHeaderToVerify, historicalBlockHeaderIndex)
	if flag != true {
		fmt.Println("error 2")
	}

	assert.True(t, flag, "Proof %v failed\n")
}

func TestGetHistoricalSummariesBlockRootsProofProofCapellaAgainstDeneb(t *testing.T) {

	//curl -H "Accept: application/json" https://data.spiceai.io/goerli/beacon/eth/v2/debug/beacon/states/7431952 -o deneb_goerli_slot_7431952.json --header 'X-API-Key: 343035|8b6ddd9b31f54c07b3fc18282b30f61c'
	currentBeaconStateJSON, err := parseJSONFile("data/deneb_goerli_slot_7431952.json")

	if err != nil {
		fmt.Println("error parsing currentBeaconStateJSON")
	}

	//this is not the beacon state of the slot containing the old withdrawal we want to proof but rather
	// its the state that was merklized to create a historical summary containing the slot that has that withdrawal, ie, 7421952 mod 8192 = 0
	oldBeaconStateJSON, err := parseJSONFile("data/goerli_slot_6397952.json.json")
	if err != nil {
		fmt.Println("error parsing oldBeaconStateJSON")
	}

	var blockHeader phase0.BeaconBlockHeader
	//blockHeader, err = ExtractBlockHeader("data/goerli_block_header_6397852.json")
	blockHeader, err = ExtractBlockHeader("data/deneb_goerli_block_header_7421951.json")

	if err != nil {
		fmt.Println("blockHeader.UnmarshalJSON error", err)
	}

	var currentBeaconState deneb.BeaconState
	var oldBeaconState deneb.BeaconState

	ParseDenebBeaconStateFromJSON(*currentBeaconStateJSON, &currentBeaconState)
	ParseDenebBeaconStateFromJSON(*oldBeaconStateJSON, &oldBeaconState)
	fmt.Println("currentBeacon state historical summary lentgh is", len(currentBeaconState.HistoricalSummaries))

	currentBeaconStateTopLevelRoots, _ := ComputeBeaconStateTopLevelRoots(&currentBeaconState)
	//oldBeaconStateTopLevelRoots, _ := ComputeBeaconStateTopLevelRoots(&oldBeaconState)

	if err != nil {
		fmt.Println("error")
	}

	historicalSummaryIndex := uint64(271)
	beaconBlockHeaderToVerifyIndex = 8191 //(7421951 mod 8192)
	beaconBlockHeaderToVerify, err := blockHeader.HashTreeRoot()
	if err != nil {
		fmt.Println("error", err)
	}

	// fmt.Println("THESE SHOULD BE", hex.EncodeToString(beaconBlockHeaderToVerify[:]))
	// fmt.Println("THE SAME", hex.EncodeToString(beaconBlockHeaderToVerify[:]))
	// fmt.Println("THESE SHOULD BE", hex.EncodeToString(oldBeaconStateTopLevelRoots.BlockRootsRoot[:]))
	// fmt.Println("THE SAME", hex.EncodeToString(currentBeaconState.HistoricalSummaries[146].BlockSummaryRoot[:]))

	oldBlockRoots := oldBeaconState.BlockRoots

	historicalSummaryBlockHeaderProof, err := ProveBlockRootAgainstBeaconStateViaHistoricalSummaries(
		currentBeaconStateTopLevelRoots,
		currentBeaconState.HistoricalSummaries,
		oldBlockRoots,
		historicalSummaryIndex,
		beaconBlockHeaderToVerifyIndex,
	)

	if err != nil {
		fmt.Println("error")
	}

	currentBeaconStateRoot, _ := currentBeaconState.HashTreeRoot()

	historicalBlockHeaderIndex := historicalSummaryListIndex<<((historicalSummaryListMerkleSubtreeNumLayers+1)+1+(blockRootsMerkleSubtreeNumLayers)) |
		historicalSummaryIndex<<(1+blockRootsMerkleSubtreeNumLayers) |
		blockSummaryRootIndex<<(blockRootsMerkleSubtreeNumLayers) | beaconBlockHeaderToVerifyIndex

	flag := ValidateProof(currentBeaconStateRoot, historicalSummaryBlockHeaderProof, beaconBlockHeaderToVerify, historicalBlockHeaderIndex)
	if flag != true {
		fmt.Println("error 2")
	}

	assert.True(t, flag, "Proof %v failed\n")

}

func TestProveValidatorAgainstValidatorList(t *testing.T) {

	// picking up one random validator index
	validatorIndex := phase0.ValidatorIndex(10000)

	// get the validators field
	validators := b.Validators

	// get the Merkle proof for inclusion
	validatorProof, err := epp.ProveValidatorAgainstValidatorList(0, validators, uint64(validatorIndex))
	if err != nil {
		fmt.Println("error")
	}

	// verify the proof
	// get the leaf corresponding to validatorIndex
	leaf, err := validators[validatorIndex].HashTreeRoot()
	if err != nil {
		fmt.Println("error")
	}

	// get the oracle state root for a merkle tree with top level roots as the leaves
	beaconStateTopLevelRoots, err := ComputeBeaconStateTopLevelRoots(&b)
	if err != nil {
		fmt.Println("error")
	}

	// calling the proof verification func
	flag := ValidateProof(*beaconStateTopLevelRoots.ValidatorsRoot, validatorProof, leaf, uint64(validatorIndex))
	if flag != true {
		fmt.Println("error")
	}
	// fmt.Println("flag", flag)

	assert.True(t, flag, "Proof %v failed\n")
}

func TestComputeBlockSlotProof(t *testing.T) {
	// get the proof for slot in the block header
	blockHeaderSlotProof, err := ProveSlotAgainstBlockHeader(&blockHeader)
	if err != nil {
		fmt.Println("error", err)
	}

	// get the hash of the slot - this will be the leaf of the merkle tree
	var slotHashRoot phase0.Root
	hh := ssz.NewHasher()
	hh.PutUint64(uint64(blockHeader.Slot))
	copy(slotHashRoot[:], hh.Hash())

	// get the block header root which will be used as a root of the Merkle tree
	// Note that the blockHeader was obtained from the actual Block header
	beaconBlockHeaderRoot, err := blockHeader.HashTreeRoot()
	if err != nil {
		fmt.Println("error:", err)
	}

	// calling the proof verification function
	flag := ValidateProof(beaconBlockHeaderRoot, blockHeaderSlotProof, slotHashRoot, slotIndex)
	if flag != true {
		fmt.Println("error")
	}

	assert.True(t, flag, "Proof %v failed\n")
}

func TestProveBlockBodyAgainstBlockHeader(t *testing.T) {

	// get the proof for block body in the block header
	blockHeaderBlockBodyProof, err := ProveBlockBodyAgainstBlockHeader(&blockHeader)
	if err != nil {
		fmt.Println("error", err)
	}

	// get the hash of the block body root - this will be the leaf of the merkle tree
	var blockBodyHashRoot phase0.Root
	hh := ssz.NewHasher()
	hh.PutBytes(blockHeader.BodyRoot[:])
	copy(blockBodyHashRoot[:], hh.Hash())

	// get the block header root which will be used as a root of the Merkle tree
	// Note that the blockHeader was obtained from the actual Block header
	beaconBlockHeaderRoot, err := blockHeader.HashTreeRoot()
	if err != nil {
		fmt.Println("error:", err)
	}

	// calling the proof verification function
	flag := ValidateProof(beaconBlockHeaderRoot, blockHeaderBlockBodyProof, blockBodyHashRoot, beaconBlockBodyRootIndex)
	if flag != true {
		fmt.Println("error")
	}

	assert.True(t, flag, "Proof %v failed\n")
}

func TestComputeExecutionPayloadHeader(t *testing.T) {

	// get the proof for execution payload in the block body
	beaconBlockBodyProof, _, err := ProveExecutionPayloadAgainstBlockBody(block.Body)
	if err != nil {
		fmt.Println("error", err)
	}

	// get the hash root of the actual execution payload
	var executionPayloadHashRoot phase0.Root
	hh := ssz.NewHasher()
	{
		if err = block.Body.ExecutionPayload.HashTreeRootWith(hh); err != nil {
			fmt.Println("error", err)
		}
		copy(executionPayloadHashRoot[:], hh.Hash())
	}

	// get the body root in the beacon block header -  will be used as the Merkle root
	blockHeaderBodyRoot := blockHeader.BodyRoot

	// calling the proof verification function
	flag := ValidateProof(blockHeaderBodyRoot, beaconBlockBodyProof, executionPayloadHashRoot, executionPayloadIndex)
	if flag != true {
		fmt.Println("error")
	}

	assert.True(t, flag, "Proof %v failed\n")
}

func TestStateRootAgainstLatestBlockHeaderProof(t *testing.T) {

	// this is the state where the latest block header from the oracle was taken.  This is the next slot after
	// the state we want to prove things about (remember latestBlockHeader.state_root = previous slot's state root)
	// oracleStateJSON, err := parseJSONFile("data/historical_summary_proof/goerli_slot_6399999.json")
	// var oracleState deneb.BeaconState
	// ParseCapellaBeaconStateFromJSON(*oracleStateJSON, &oracleState)

	var blockHeader phase0.BeaconBlockHeader
	blockHeader, err := ExtractBlockHeader("data/deneb_goerli_block_header_7413760.json")
	if err != nil {
		fmt.Println("error with block header", err)
	}

	//the state from the prev slot which contains shit we wanna prove about
	stateToProveJSON, err := parseJSONFile("data/deneb_goerli_slot_7413760.json")

	var stateToProve deneb.BeaconState
	ParseDenebBeaconStateFromJSON(*stateToProveJSON, &stateToProve)

	roots, _ := stateToProve.HashTreeRoot()
	fmt.Println("THIS IS ROOT", roots)
	proof, err := ProveStateRootAgainstBlockHeader(&blockHeader)
	if err != nil {
		fmt.Println("Error in generating proof", err)
	}

	fmt.Println(len(stateToProve.Validators))

	root, err := blockHeader.HashTreeRoot()
	if err != nil {
		fmt.Println("this error", err)
	}
	leaf, err := stateToProve.HashTreeRoot()
	if err != nil {
		fmt.Println("this error", err)
	}

	flag := ValidateProof(root, proof, leaf, 3)
	if flag != true {
		fmt.Println("this error")
	}
	assert.True(t, flag, "Proof %v failed")
}

func TestGetExecutionPayloadProof(t *testing.T) {

	// get the proof for execution payload in the block body

	exectionPayloadProof, _, _ := ProveExecutionPayloadAgainstBlockHeader(&blockHeader, block.Body)

	// get the hash root of the actual execution payload
	var executionPayloadHashRoot, _ = block.Body.ExecutionPayload.HashTreeRoot()

	// get the body root in the beacon block header -  will be used as the Merkle root
	root, _ := blockHeader.HashTreeRoot()

	index := beaconBlockBodyRootIndex<<(blockBodyMerkleSubtreeNumLayers) | executionPayloadIndex

	// calling the proof verification function
	flag := ValidateProof(root, exectionPayloadProof, executionPayloadHashRoot, index)
	if flag != true {
		fmt.Println("error")
	}

	assert.True(t, flag, "Proof %v failed")
}

func TestComputeWithdrawalsListProof(t *testing.T) {

	withdrawalsListProof, err := ProveWithdrawalListAgainstExecutionPayload(block.Body.ExecutionPayload)
	if err != nil {
		fmt.Println("error!", err)
	}

	var withdrawalsHashRoot phase0.Root
	hh := ssz.NewHasher()

	{
		subIndx := hh.Index()
		num := uint64(len(block.Body.ExecutionPayload.Withdrawals))
		if num > 16 {
			err := ssz.ErrIncorrectListSize
			fmt.Println("error!", err)
		}
		for _, elem := range block.Body.ExecutionPayload.Withdrawals {
			if err = elem.HashTreeRootWith(hh); err != nil {
				fmt.Println("error 4", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, num, 16)
		copy(withdrawalsHashRoot[:], hh.Hash())
		hh.Reset()
	}

	var executionPayloadHashRoot phase0.Root
	{
		if err = block.Body.ExecutionPayload.HashTreeRootWith(hh); err != nil {
			fmt.Println("error hel", err)
		}
		copy(executionPayloadHashRoot[:], hh.Hash())
	}
	flag := ValidateProof(executionPayloadHashRoot, withdrawalsListProof, withdrawalsHashRoot, withdrawalsIndex)
	if flag != true {
		fmt.Println("Proof Failed")
	}
	assert.True(t, flag, "Proof %v failed\n")

}

func TestComputeIndividualWithdrawalProof(t *testing.T) {

	// picking up one random validator index
	withdrawalIndex := uint8(0)

	// get the validators field
	withdrawals := block.Body.ExecutionPayload.Withdrawals

	// get the Merkle proof for inclusion
	withdrawalProof, err := ProveWithdrawalAgainstWithdrawalList(withdrawals, withdrawalIndex)
	if err != nil {
		fmt.Println("error")
	}

	// verify the proof
	// get the leaf corresponding to validatorIndex
	leaf, err := withdrawals[withdrawalIndex].HashTreeRoot()
	if err != nil {
		fmt.Println("error")
	}

	var withdrawalsHashRoot phase0.Root
	hh := ssz.NewHasher()

	{
		subIndx := hh.Index()
		num := uint64(len(block.Body.ExecutionPayload.Withdrawals))
		if num > 16 {
			err := ssz.ErrIncorrectListSize
			fmt.Println("error", err)
		}
		for _, elem := range block.Body.ExecutionPayload.Withdrawals {
			if err = elem.HashTreeRootWith(hh); err != nil {
				fmt.Println("error", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, num, 16)
		copy(withdrawalsHashRoot[:], hh.Hash())
		hh.Reset()
	}

	// calling the proof verification func
	flag := ValidateProof(withdrawalsHashRoot, withdrawalProof, leaf, uint64(withdrawalIndex))
	if flag != true {
		fmt.Println("error")
	}

	assert.True(t, flag, "Proof %v failed\n")
}

func TestGetWithdrawalProof(t *testing.T) {

	// picking up one random validator index
	withdrawalIndex := uint8(0)

	withdrawalProof, _ := ProveWithdrawalAgainstExecutionPayload(block.Body.ExecutionPayload, withdrawalIndex)

	executionPayloadRoot, _ := block.Body.ExecutionPayload.HashTreeRoot()

	leaf, err := block.Body.ExecutionPayload.Withdrawals[withdrawalIndex].HashTreeRoot()
	if err != nil {
		fmt.Println("error")
	}
	// withdrawalIndex = beaconBlockBodyRootIndex<<( blockBodyMerkleSubtreeNumLayers+ executionPayloadMerkleSubtreeNumLayers+( withdrawalListMerkleSubtreeNumLayers+1)) | executionPayloadIndex<<( executionPayloadMerkleSubtreeNumLayers+( withdrawalListMerkleSubtreeNumLayers+1)) | withdrawalsIndex<<( withdrawalListMerkleSubtreeNumLayers+1) | withdrawalIndex

	withdrawalRelativeToELPayloadIndex := withdrawalsIndex<<(withdrawalListMerkleSubtreeNumLayers+1) | uint64(withdrawalIndex)

	// calling the proof verification func
	flag := ValidateProof(executionPayloadRoot, withdrawalProof, leaf, withdrawalRelativeToELPayloadIndex)
	if flag != true {
		fmt.Println("error")
	}

	assert.True(t, flag, "Proof %v failed\n")
}

func TestGetTimestampProof(t *testing.T) {

	// get the block number
	executionPayloadFields := block.Body.ExecutionPayload

	// get the Merkle proof for inclusion
	timestampProof, _ := ProveTimestampAgainstExecutionPayload(executionPayloadFields)

	hh := ssz.NewHasher()
	hh.PutUint64(uint64(executionPayloadFields.Timestamp))

	leaf := ConvertTo32ByteArray(hh.Hash())

	root, err := block.Body.ExecutionPayload.HashTreeRoot()
	if err != nil {
		fmt.Println("error")
	}

	// calling the proof verification func
	flag := ValidateProof(root, timestampProof, leaf, timestampIndex)
	if flag != true {
		fmt.Println("proof failed")
	}

	assert.True(t, flag, "Proof %v failed")
}

func TestGetValidatorProof(t *testing.T) {
	// picking up one random validator index
	validatorIndex := uint64(VALIDATOR_INDEX)

	// get the validators field
	validators := b.Validators

	beaconStateTopLevelRoots, err := ComputeBeaconStateTopLevelRoots(&b)

	validatorProof, _ := epp.ProveValidatorAgainstBeaconState(&b, beaconStateTopLevelRoots, uint64(validatorIndex))

	// verify the proof
	// get the leaf corresponding to validatorIndex
	leaf, err := validators[validatorIndex].HashTreeRoot()
	if err != nil {
		fmt.Println("error")
	}

	// calling the proof verification func
	beaconRoot, _ := b.HashTreeRoot()

	validatorIndex = validatorListIndex<<(validatorListMerkleSubtreeNumLayers+1) | uint64(validatorIndex)

	flag := ValidateProof(beaconRoot, validatorProof, leaf, validatorIndex)
	if flag != true {
		fmt.Println("error")
	}

	assert.True(t, flag, "Proof %v failed\n")
}

func TestGetSlotProof(t *testing.T) {
	// picking up one random validator index
	slot := blockHeader.Slot

	buf := make([]byte, 32)
	binary.LittleEndian.PutUint64(buf, uint64(slot))
	var bytes32 [32]byte
	copy(bytes32[:], buf[:32])

	proof, _ := ProveSlotAgainstBlockHeader(&blockHeader)

	root, _ := blockHeader.HashTreeRoot()

	hh := ssz.NewHasher()
	hh.PutUint64(uint64(slot))

	leaf := ConvertTo32ByteArray(hh.Hash())

	flag := ValidateProof(root, proof, leaf, 0)
	if flag != true {
		fmt.Println("error")
	}
	assert.True(t, flag, "Proof %v failed\n")
}

type Proofs struct {
	Slot                  uint64   `json:"slot"`
	ValidatorIndex        uint64   `json:"validatorIndex"`
	WithdrawalIndex       uint64   `json:"withdrawalIndex"`
	BlockHeaderRootIndex  uint64   `json:"blockHeaderRootIndex"`
	BeaconStateRoot       string   `json:"beaconStateRoot"`
	SlotRoot              string   `json:"slotRoot"`
	BlockNumberRoot       string   `json:"blockNumberRoot"`
	BlockHeaderRoot       string   `json:"blockHeaderRoot"`
	BlockBodyRoot         string   `json:"blockBodyRoot"`
	ExecutionPayloadRoot  string   `json:"executionPayloadRoot"`
	BlockHeaderProof      []string `json:"BlockHeaderProof"`
	SlotProof             []string `json:"SlotProof"`
	WithdrawalProof       []string `json:"WithdrawalProof"`
	ValidatorProof        []string `json:"ValidatorProof"`
	BlockNumberProof      []string `json:"BlockNumberProof"`
	ExecutionPayloadProof []string `json:"ExecutionPayloadProof"`
	ValidatorFields       []string `json:"ValidatorFields"`
	WithdrawalFields      []string `json:"WithdrawalFields"`
}

func parseJSONFile(filePath string) (*beaconStateJSON, error) {
	data, err := os.ReadFile(filePath)

	if err != nil {
		fmt.Println("error with reading file")
		return nil, err
	}

	var beaconState beaconStateVersion
	err = json.Unmarshal(data, &beaconState)
	if err != nil {
		fmt.Println("error with beaconState JSON unmarshalling")
		return nil, err
	}

	actualData := beaconState.Data
	return &actualData, nil
}
