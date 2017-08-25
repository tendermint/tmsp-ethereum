package app_test

import (
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"golang.org/x/net/context"

	ethUtils "github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/tendermint/ethermint/app"
	emtUtils "github.com/tendermint/ethermint/cmd/utils"
	"github.com/tendermint/ethermint/ethereum"

	abciTypes "github.com/tendermint/abci/types"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	tmLog "github.com/tendermint/tmlibs/log"
)

var (
	receiverAddress = common.StringToAddress("0x1234123412341234123412341234123412341234")
)

// implements: tendermint.rpc.client.HTTPClient
type MockClient struct {
	sentBroadcastTx chan struct{} // fires when we call broadcast_tx_sync
}

func NewMockClient() *MockClient { return &MockClient{make(chan struct{})} }

func (mc *MockClient) Call(method string, params map[string]interface{}, result interface{}) (interface{}, error) {
	_ = result
	switch method {
	case "status":
		result = &ctypes.ResultStatus{}
		return result, nil
	case "broadcast_tx_sync":
		close(mc.sentBroadcastTx)
		result = &ctypes.ResultBroadcastTx{}
		return result, nil
	}

	return nil, abciTypes.ErrInternalError
}

func setupTestCase(t *testing.T) (func(t *testing.T), string) {
	t.Log("Setup test case")
	temporaryDirectory, err := ioutil.TempDir("", "ethermint_test")
	if err != nil {
		t.Error("Unable to create the temporary directory for the tests.")
	}
	return func(t *testing.T) {
		t.Log("Tearing down test case")
		os.RemoveAll(temporaryDirectory)
	}, temporaryDirectory
}

func TestBumpingNonces(t *testing.T) {
	teardownTestCase, temporaryDirectory := setupTestCase(t)
	defer teardownTestCase(t)

	// generate key
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		t.Errorf("Error generating key %v", err)
	}
	addr := crypto.PubkeyToAddress(privateKey.PublicKey)
	ctx := context.Background()

	// used to intercept rpc calls to tendermint
	mockclient := NewMockClient()

	stack, backend, app, err := makeTestApp(temporaryDirectory, []common.Address{addr}, mockclient)
	if err != nil {
		t.Errorf("Error making test EthermintApplication: %v", err)
	}

	// first transaction is sent via ABCI by us pretending to be Tendermint, should pass
	height := uint64(1)
	nonce1 := uint64(0)
	tx1, err := createTransaction(privateKey, nonce1, receiverAddress)
	if err != nil {
		t.Errorf("Error creating transaction: %v", err)

	}
	encodedtx, _ := rlp.EncodeToBytes(tx1)

	// check transaction
	assert.Equal(t, abciTypes.OK, app.CheckTx(encodedtx))

	// set time greater than time of prev tx (zero)
	app.BeginBlock([]byte{}, &abciTypes.Header{Height: height, Time: 1})

	// check deliverTx
	assert.Equal(t, abciTypes.OK, app.DeliverTx(encodedtx))

	app.EndBlock(height)

	// check commit
	assert.Equal(t, abciTypes.OK.Code, app.Commit().Code)

	// replays should fail - we're checking if the transaction got through earlier, by replaying the nonce
	assert.Equal(t, abciTypes.ErrBadNonce.Code, app.CheckTx(encodedtx).Code)

	// ...on both interfaces of the app
	assert.Equal(t, core.ErrNonceTooLow, backend.Ethereum().ApiBackend.SendTx(ctx, tx1))

	// second transaction is sent via geth RPC, or at least pretending to be so
	// with a correct nonce this time, it should pass
	nonce2 := uint64(1)
	tx2, _ := createTransaction(privateKey, nonce2, receiverAddress)

	assert.Equal(t, backend.Ethereum().ApiBackend.SendTx(ctx, tx2), nil)

	ticker := time.NewTicker(5 * time.Second)
	select {
	case <-ticker.C:
		assert.Fail(t, "Timeout waiting for transaction on the tendermint rpc")
	case <-mockclient.sentBroadcastTx:
	}

	stack.Stop() // nolint: errcheck
}

// TestMultipleTxOneAcc sends multiple TXs from the same account in the same block
func TestMultipleTxOneAcc(t *testing.T) {
	teardownTestCase, temporaryDirectory := setupTestCase(t)
	defer teardownTestCase(t)

	// generate key
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		t.Errorf("Error generating key %v", err)
	}
	addr := crypto.PubkeyToAddress(privateKey.PublicKey)

	// used to intercept rpc calls to tendermint
	mockclient := NewMockClient()

	node, _, app, err := makeTestApp(temporaryDirectory, []common.Address{addr}, mockclient)
	if err != nil {
		t.Errorf("Error making test EthermintApplication: %v", err)
	}

	// first transaction is sent via ABCI by us pretending to be Tendermint, should pass
	height := uint64(1)

	nonce1 := uint64(0)
	tx1, err := createTransaction(privateKey, nonce1, receiverAddress)
	if err != nil {
		t.Errorf("Error creating transaction: %v", err)

	}
	encodedTx1, _ := rlp.EncodeToBytes(tx1)

	//create 2-nd tx from the same account
	nonce2 := uint64(1)
	tx2, err := createTransaction(privateKey, nonce2, receiverAddress)
	if err != nil {
		t.Errorf("Error creating transaction: %v", err)

	}
	encodedTx2, _ := rlp.EncodeToBytes(tx2)

	// check transaction
	assert.Equal(t, abciTypes.OK, app.CheckTx(encodedTx1))

	//check tx on 2nd tx should pass until we implement state in CheckTx
	assert.Equal(t, abciTypes.OK, app.CheckTx(encodedTx2))

	// set time greater than time of prev tx (zero)
	app.BeginBlock([]byte{}, &abciTypes.Header{Height: height, Time: 1})

	// check deliverTx for 1st tx
	assert.Equal(t, abciTypes.OK, app.DeliverTx(encodedTx1))

	// and for 2nd tx (should fail because of wrong nonce2)

	assert.Equal(t, abciTypes.OK, app.DeliverTx(encodedTx2))

	app.EndBlock(height)

	// check commit
	assert.Equal(t, abciTypes.OK.Code, app.Commit().Code)

	node.Stop() // nolint: errcheck
}

func TestMultipleTxTwoAcc(t *testing.T) {
	teardownTestCase, temporaryDirectory := setupTestCase(t)
	defer teardownTestCase(t)

	// generate key
	//account 1
	privateKey1, err := crypto.GenerateKey()
	if err != nil {
		t.Errorf("Error generating key %v", err)
	}
	addr1 := crypto.PubkeyToAddress(privateKey1.PublicKey)

	//account 2
	privateKey2, err := crypto.GenerateKey()
	if err != nil {
		t.Errorf("Error generating key %v", err)
	}
	addr2 := crypto.PubkeyToAddress(privateKey2.PublicKey)

	// used to intercept rpc calls to tendermint
	mockclient := NewMockClient()

	node, _, app, err := makeTestApp(temporaryDirectory, []common.Address{addr1, addr2}, mockclient)
	if err != nil {
		t.Errorf("Error making test EthermintApplication: %v", err)
	}

	// first transaction is sent via ABCI by us pretending to be Tendermint, should pass
	height := uint64(1)
	nonce1 := uint64(0)
	tx1, err := createTransaction(privateKey1, nonce1, receiverAddress)
	if err != nil {
		t.Errorf("Error creating transaction: %v", err)

	}
	encodedTx1, _ := rlp.EncodeToBytes(tx1)

	//create 2-nd tx
	nonce2 := uint64(0)
	tx2, err := createTransaction(privateKey2, nonce2, receiverAddress)
	if err != nil {
		t.Errorf("Error creating transaction: %v", err)

	}
	encodedTx2, _ := rlp.EncodeToBytes(tx2)

	// check transaction
	assert.Equal(t, abciTypes.OK, app.CheckTx(encodedTx1))
	assert.Equal(t, abciTypes.OK, app.CheckTx(encodedTx2))

	// set time greater than time of prev tx (zero)
	app.BeginBlock([]byte{}, &abciTypes.Header{Height: height, Time: 1})

	// check deliverTx for 1st tx
	assert.Equal(t, abciTypes.OK, app.DeliverTx(encodedTx1))
	// and for 2nd tx
	assert.Equal(t, abciTypes.OK, app.DeliverTx(encodedTx2))

	app.EndBlock(height)

	// check commit
	assert.Equal(t, abciTypes.OK.Code, app.Commit().Code)

	node.Stop() // nolint: errcheck
}

// Test transaction from Acc1 to new Acc2 and then from Acc2 to another address
// in the same block
func TestFromAccToAcc(t *testing.T) {
	teardownTestCase, temporaryDirectory := setupTestCase(t)
	defer teardownTestCase(t)

	// generate key
	//account 1
	privateKey1, err := crypto.GenerateKey()
	if err != nil {
		t.Errorf("Error generating key %v", err)
	}
	addr1 := crypto.PubkeyToAddress(privateKey1.PublicKey)

	//account 2
	privateKey2, err := crypto.GenerateKey()
	if err != nil {
		t.Errorf("Error generating key %v", err)
	}
	addr2 := crypto.PubkeyToAddress(privateKey2.PublicKey)

	// used to intercept rpc calls to tendermint
	mockclient := NewMockClient()

	// initialize ethermint only with account 1
	node, _, app, err := makeTestApp(temporaryDirectory, []common.Address{addr1}, mockclient)
	if err != nil {
		t.Errorf("Error making test EthermintApplication: %v", err)
	}

	// first transaction from Acc1 to Acc2 (which is not in genesis)
	height := uint64(1)

	nonce1 := uint64(0)
	tx1, err := createCustomTransaction(privateKey1, nonce1, addr2, big.NewInt(1000000), big.NewInt(21000), big.NewInt(10))
	if err != nil {
		t.Errorf("Error creating transaction: %v", err)

	}
	encodedTx1, _ := rlp.EncodeToBytes(tx1)

	// second transaction from Acc2 to some another address
	nonce2 := uint64(0)
	tx2, err := createCustomTransaction(privateKey2, nonce2, receiverAddress, big.NewInt(2), big.NewInt(21000), big.NewInt(10))
	if err != nil {
		t.Errorf("Error creating transaction: %v", err)

	}
	encodedTx2, _ := rlp.EncodeToBytes(tx2)

	// check transaction
	assert.Equal(t, abciTypes.OK, app.CheckTx(encodedTx1))

	// check tx2
	assert.Equal(t, abciTypes.OK, app.CheckTx(encodedTx2))

	// set time greater than time of prev tx (zero)
	app.BeginBlock([]byte{}, &abciTypes.Header{Height: height, Time: 1})

	// check deliverTx for 1st tx
	assert.Equal(t, abciTypes.OK, app.DeliverTx(encodedTx1))

	// and for 2nd tx
	assert.Equal(t, abciTypes.OK, app.DeliverTx(encodedTx2))

	app.EndBlock(height)

	// check commit
	assert.Equal(t, abciTypes.OK.Code, app.Commit().Code)

	node.Stop() // nolint: errcheck
}

// 1. put Acc1 and Acc2 to genesis with some amounts (X)
// 2. transfer 10 amount from Acc1 to Acc2
// 3. in the same block transfer from Acc2 to another Acc all his amounts (X+10)
func TestFromAccToAcc2(t *testing.T) {
	teardownTestCase, temporaryDirectory := setupTestCase(t)
	defer teardownTestCase(t)

	// generate key
	//account 1
	privateKey1, err := crypto.GenerateKey()
	if err != nil {
		t.Errorf("Error generating key %v", err)
	}
	addr1 := crypto.PubkeyToAddress(privateKey1.PublicKey)

	//account 2
	privateKey2, err := crypto.GenerateKey()
	if err != nil {
		t.Errorf("Error generating key %v", err)
	}
	addr2 := crypto.PubkeyToAddress(privateKey2.PublicKey)

	// used to intercept rpc calls to tendermint
	mockclient := NewMockClient()

	node, _, app, err := makeTestApp(temporaryDirectory, []common.Address{addr1, addr2}, mockclient)
	if err != nil {
		t.Errorf("Error making test EthermintApplication: %v", err)
	}

	height := uint64(1)

	nonce1 := uint64(0)
	tx1, err := createCustomTransaction(privateKey1, nonce1, addr2, big.NewInt(500000), big.NewInt(21000), big.NewInt(10))
	if err != nil {
		t.Errorf("Error creating transaction: %v", err)

	}
	encodedTx1, _ := rlp.EncodeToBytes(tx1)

	// second transaction from Acc2 to some another address
	nonce2 := uint64(0)
	//here initial value + 10
	tx2, err := createCustomTransaction(privateKey2, nonce2, receiverAddress, big.NewInt(1000000), big.NewInt(21000), big.NewInt(10))
	if err != nil {
		t.Errorf("Error creating transaction: %v", err)

	}
	encodedTx2, _ := rlp.EncodeToBytes(tx2)

	// check transaction
	assert.Equal(t, abciTypes.OK, app.CheckTx(encodedTx1))

	// check tx2
	assert.Equal(t, abciTypes.OK, app.CheckTx(encodedTx2))

	// set time greater than time of prev tx (zero)
	app.BeginBlock([]byte{}, &abciTypes.Header{Height: height, Time: 1})

	// check deliverTx for 1st tx
	assert.Equal(t, abciTypes.OK, app.DeliverTx(encodedTx1))

	// and for 2nd tx
	assert.Equal(t, abciTypes.OK, app.DeliverTx(encodedTx2))

	app.EndBlock(height)

	// check commit
	assert.Equal(t, abciTypes.OK.Code, app.Commit().Code)

	node.Stop() // nolint: errcheck
}

// mimics abciEthereumAction from cmd/ethermint/main.go
func makeTestApp(tempDatadir string, addresses []common.Address, mockclient *MockClient) (*node.Node, *ethereum.Backend, *app.EthermintApplication, error) {
	stack, err := makeTestSystemNode(tempDatadir, addresses, mockclient)
	if err != nil {
		return nil, nil, nil, err
	}
	ethUtils.StartNode(stack)

	var backend *ethereum.Backend
	if err = stack.Service(&backend); err != nil {
		return nil, nil, nil, err
	}

	app, err := app.NewEthermintApplication(backend, nil, nil)
	app.SetLogger(tmLog.TestingLogger())

	return stack, backend, app, err
}

// mimics MakeSystemNode from ethereum/node.go
func makeTestSystemNode(tempDatadir string, addresses []common.Address, mockclient *MockClient) (*node.Node, error) {
	// Configure the node's service container
	nodeConf := emtUtils.DefaultNodeConfig()
	emtUtils.SetEthermintNodeConfig(&nodeConf)
	nodeConf.DataDir = tempDatadir

	// Configure the Ethereum service
	ethConf := eth.DefaultConfig
	emtUtils.SetEthermintEthConfig(&ethConf)

	genesis, err := makeTestGenesis(addresses)
	if err != nil {
		return nil, err
	}

	ethConf.Genesis = genesis

	// Assemble and return the protocol stack
	stack, err := node.New(&nodeConf)
	if err != nil {
		return nil, err
	}
	return stack, stack.Register(func(ctx *node.ServiceContext) (node.Service, error) {
		return ethereum.NewBackend(ctx, &ethConf, mockclient)
	})
}

func makeTestGenesis(addresses []common.Address) (*core.Genesis, error) {
	gopath := os.Getenv("GOPATH")
	genesisPath := filepath.Join(gopath, "src/github.com/tendermint/ethermint/setup/genesis.json")

	file, err := os.Open(genesisPath)
	if err != nil {
		return nil, err
	}
	defer file.Close() // nolint: errcheck

	genesis := new(core.Genesis)
	if err := json.NewDecoder(file).Decode(genesis); err != nil {
		ethUtils.Fatalf("invalid genesis file: %v", err)
	}

	balance, result := new(big.Int).SetString("10000000000000000000000000000000000", 10)
	if !result {
		return nil, errors.New("BigInt convertation error")
	}

	for _, addr := range addresses {
		genesis.Alloc[addr] = core.GenesisAccount{Balance: balance}
	}

	return genesis, nil
}

func createTransaction(key *ecdsa.PrivateKey, nonce uint64, to common.Address) (*types.Transaction, error) {
	signer := types.HomesteadSigner{}

	return types.SignTx(
		types.NewTransaction(nonce, to, big.NewInt(10), big.NewInt(21000), big.NewInt(10),
			nil),
		signer,
		key,
	)
}

func createCustomTransaction(key *ecdsa.PrivateKey, nonce uint64, to common.Address, amount, gasLimit, gasPrice *big.Int) (*types.Transaction, error) {
	signer := types.HomesteadSigner{}

	return types.SignTx(
		types.NewTransaction(nonce, to, amount, gasLimit, gasPrice, nil),
		signer,
		key,
	)
}
