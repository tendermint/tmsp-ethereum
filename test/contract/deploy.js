const config = require('config');
const ABI = require('./config/ABI.json');
const Web3 = require('web3');

const _web3 = new Web3(new Web3.providers.HttpProvider(config.get('provider')));
_web3._extend({
    property: 'personal',
    methods: [
        new _web3._extend.Method({
            name: 'unlockAccount',
            call: 'personal_unlockAccount',
            params: 3,
            inputFormatter: [null, null, null]
        })
    ]
});

_web3.personal.unlockAccount('0x'+config.get('wallet').address, config.get('password'));

const productsContract = _web3.eth.contract(ABI);
const products = productsContract.new(
  {
    from: _web3.eth.accounts[0],
    data: '0x6060604052341561000c57fe5b5b61075a8061001c6000396000f30060606040526000357c0100000000000000000000000000000000000000000000000000000000900463ffffffff1680631691473c14610051578063b9db15b4146100ea578063bc05d087146101cb575bfe5b341561005957fe5b610085600480803573ffffffffffffffffffffffffffffffffffffffff1690602001909190505061022e565b60405180806020018281038252838181518152602001915080519060200190602002808383600083146100d7575b8051825260208311156100d7576020820191506020810190506020830392506100b3565b5050509050019250505060405180910390f35b34156100f257fe5b61010860048080359060200190919050506102cc565b60405180848152602001806020018373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200182810382528481815181526020019150805190602001908083836000831461018f575b80518252602083111561018f5760208201915060208101905060208303925061016b565b505050905090810190601f1680156101bb5780820380516001836020036101000a031916815260200191505b5094505050505060405180910390f35b34156101d357fe5b61022c600480803590602001909190803590602001908201803590602001908080601f016020809104026020016040519081016040528093929190818152602001838380828437820191505050505050919050506103da565b005b610236610635565b600160008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020019081526020016000208054806020026020016040519081016040528092919081815260200182805480156102bf57602002820191906000526020600020905b8154815260200190600101908083116102ab575b505050505090505b919050565b60006102d6610649565b600060006000600086815260200190815260200160002090508060000154816001016000600088815260200190815260200160002060020160009054906101000a900473ffffffffffffffffffffffffffffffffffffffff16818054600181600116156101000203166002900480601f0160208091040260200160405190810160405280929190818152602001828054600181600116156101000203166002900480156103c45780601f10610399576101008083540402835291602001916103c4565b820191906000526020600020905b8154815290600101906020018083116103a757829003601f168201915b505050505091509350935093505b509193909250565b6060604051908101604052808381526020018281526020013373ffffffffffffffffffffffffffffffffffffffff168152506000600084815260200190815260200160002060008201518160000155602082015181600101908051906020019061044592919061065d565b5060408201518160020160006101000a81548173ffffffffffffffffffffffffffffffffffffffff021916908373ffffffffffffffffffffffffffffffffffffffff160217905550905050600160003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002080548060010182816104e191906106dd565b916000526020600020900160005b84909190915055506000600083815260200190815260200160002060020160009054906101000a900473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff167fadf710a8ef6202ad47d4f3fe8844c285e392dc2dbc2797dd4859dedbd1fd5e8360006000858152602001908152602001600020600001546000600086815260200190815260200160002060010160405180838152602001806020018281038252838181546001816001161561010002031660029004815260200191508054600181600116156101000203166002900480156106215780601f106105f657610100808354040283529160200191610621565b820191906000526020600020905b81548152906001019060200180831161060457829003601f168201915b5050935050505060405180910390a25b5050565b602060405190810160405280600081525090565b602060405190810160405280600081525090565b828054600181600116156101000203166002900490600052602060002090601f016020900481019282601f1061069e57805160ff19168380011785556106cc565b828001600101855582156106cc579182015b828111156106cb5782518255916020019190600101906106b0565b5b5090506106d99190610709565b5090565b815481835581811511610704578183600052602060002091820191016107039190610709565b5b505050565b61072b91905b8082111561072757600081600090555060010161070f565b5090565b905600a165627a7a72305820be1a201c1518079c9986fad0961fa10136252c5e377fbabfe6eb9ee1009e787d0029',
    gas: '4700000'
  }, (e, contract) => {
    console.log(e, contract);
    if (typeof contract.address !== 'undefined') {
      console.log('Contract mined! address: ' + contract.address + ' transactionHash: ' + contract.transactionHash);
    }
  });