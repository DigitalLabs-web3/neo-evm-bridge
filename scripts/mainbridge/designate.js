const config = require("./config");
const { rpc, sc, wallet, tx, CONST, u } = require("@cityofzion/neon-js");


async function main() {
    let ncfg = config.networks[config.defaultNetwork];
    const account = new wallet.Account(ncfg.wif);
    console.log("invoker:" + account.address);
    let scriptBuilder = new sc.ScriptBuilder();
    scriptBuilder.emitContractCall({
        scriptHash: "0x1c3ba4cfb7a9c9c1617c28d5c91160426859895f",
        operation: "designateValidators",
        callFlags: sc.CallFlags.All,
        args: [sc.ContractParam.array(
            sc.ContractParam.byteArray(u.HexString.fromHex("028e5d4f8e87e97a45c3bfc1146b8d810fbd58577a07d00c0cb7f07b78638aa637").reversed()),
            sc.ContractParam.byteArray(u.HexString.fromHex("03efb3059e7ea113f221d01ee1445ff56a14b22ceeb62ce77b98ef00eea16dbdef").reversed()),
            sc.ContractParam.byteArray(u.HexString.fromHex("022db5e20a60aff0f61e28e3ea0e42c751b611a57325c252acd7de83961f8f71a0").reversed()),
            sc.ContractParam.byteArray(u.HexString.fromHex("0351e032b71324fce6e204b8a3afafcac4f2efb21d49e124489a69b8bfb0db0948").reversed())
        )]
    });
    let client = new rpc.RPCClient(ncfg.url);
    let height = await client.getBlockCount();
    console.log(height);
    let t = new tx.Transaction({
        version: CONST.TX_VERSION,
        validUntilBlock: height + 1000,
        signers: [{ account: account.scriptHash, scopes: tx.WitnessScope.CalledByEntry }],
        witnesses: [{ verificationScript: wallet.getVerificationScriptFromPublicKey(account.getPublicKey()), invocationScript: "" }],
        script: scriptBuilder.build(),
    });
    let invokeResult = await client.invokeScript(t.script, [{ account: account.address, scopes: tx.WitnessScope.CalledByEntry }]);
    t.systemFee = u.BigInteger.fromNumber(invokeResult.gasconsumed);
    let networkFee = await client.calculateNetworkFee(t);
    t.networkFee = u.BigInteger.fromNumber(networkFee);
    t.sign(account, CONST.MAGIC_NUMBER.TestNet);

    let txid = await client.sendRawTransaction(t);
    console.log("txid: " + txid);
}

main().then(() => process.exit(0)).catch(console.log);
