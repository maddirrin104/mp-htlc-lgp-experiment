// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "forge-std/Script.sol";
import {MockToken} from "../contracts/MockToken.sol";
import {MPHTLC_LGP} from "../contracts/MPHTLC_LGP.sol";

contract Deploy is Script {
    function run() external {
        uint256 deployerPk = vm.envUint("DEPLOYER_PK");
        string memory outPath = "configs/deployed.json";
        // allow override
        try vm.envString("DEPLOY_OUT") returns (string memory p) {
            if (bytes(p).length != 0) outPath = p;
        } catch {}

        vm.startBroadcast(deployerPk);
        MockToken token = new MockToken("MockToken", "MTK");
        MPHTLC_LGP htlc = new MPHTLC_LGP();
        vm.stopBroadcast();

        string memory obj = "deploy";
        vm.serializeAddress(obj, "token", address(token));
        string memory json = vm.serializeAddress(obj, "htlc", address(htlc));
        vm.writeJson(json, outPath);

        console2.log("TOKEN:", address(token));
        console2.log("HTLC:", address(htlc));
        console2.log("Wrote:", outPath);
    }
}
