// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "forge-std/Script.sol";
import {MPHTLCLGP} from "../src/MPHTLCLGP.sol";

contract DeployMPHTLCLGP is Script {
    function run() external returns (MPHTLCLGP deployed) {
        uint256 deployerKey = vm.envUint("PRIVATE_KEY");

        vm.startBroadcast(deployerKey);
        deployed = new MPHTLCLGP();
        vm.stopBroadcast();
    }
}
