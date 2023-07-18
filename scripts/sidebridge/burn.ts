import {ethers} from "ethers"

const abi = ["function burn(address) public payable"]
  
async function burn() {
    const key = ""
    const provider = new ethers.JsonRpcProvider("http://localhost:31332")
    const wallet = new ethers.Wallet(key,provider)
    
    const bridge = new ethers.Contract("0x00000000000000000000000000000000000000e5", abi, wallet)
    let tx = await bridge.burn("0xe7bfb539b6003cae46bceb3a48b948274e51d606", {value: ethers.parseEther("2")}) // scripthash in neo-cli 0x06d6514e2748b9483aebbc46ae3c00b639b5bfe7
    let r = await tx.wait()
    console.log(r)
}

burn().then(() => process.exit(0)).catch(console.log)
