const config = require("./config");
const { readFileSync } = require("node:fs")
const { experimental, CONST, sc, wallet } = require("@cityofzion/neon-js");


async function main() {
    let ncfg = config.networks[config.defaultNetwork];

    const account = new wallet.Account(ncfg.wif);
    console.log("deployer:" + account.address);
    let bb = await readFileSync("../contract/Contract/bin/sc/BridgeContract.nef");
    let nef = sc.NEF.fromBuffer(bb);
    let manifestJson = require("../contract/Contract/bin/sc/BridgeContract.manifest.json");
    let manifest = sc.ContractManifest.fromJson(manifestJson);
    let r = await experimental.deployContract(nef, manifest, {
        networkMagic: CONST.MAGIC_NUMBER.TestNet,
        rpcAddress: ncfg.url,
        account: account,
    });
    console.log(r);
}

main().then(() => process.exit(0)).catch(console.log);
