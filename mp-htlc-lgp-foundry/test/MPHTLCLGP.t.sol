// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "forge-std/Test.sol";
import {MPHTLCLGP} from "../src/MPHTLCLGP.sol";

contract MPHTLCLGPTest is Test {
    MPHTLCLGP internal htlc;

    uint256 internal senderPk = 0xA11CE;
    uint256 internal receiverPk = 0xB0B;
    uint256 internal authorityPk = 0xCAFE;
    uint256 internal attackerPk = 0xBAD;

    address internal sender;
    address internal receiver;
    address internal authority;
    address internal attacker;

    uint256 internal constant AMOUNT = 10 ether;
    uint256 internal constant DEPOSIT = 2 ether;
    uint64 internal constant DEPOSIT_DELAY = 600;
    uint64 internal constant TIMELOCK_DELAY = 1800;
    uint64 internal constant PENALTY_WINDOW = 600;

    bytes32 internal constant PREIMAGE = keccak256("demo-preimage");
    bytes32 internal constant HASHLOCK = keccak256(abi.encodePacked(PREIMAGE));

    function setUp() public {
        sender = vm.addr(senderPk);
        receiver = vm.addr(receiverPk);
        authority = vm.addr(authorityPk);
        attacker = vm.addr(attackerPk);

        vm.deal(sender, 100 ether);
        vm.deal(receiver, 100 ether);
        vm.deal(attacker, 100 ether);

        htlc = new MPHTLCLGP();
    }

    function test_S1_ClaimEarly_NoPenalty() public {
        bytes32 swapId = _openAndDeposit(3);
        vm.warp(block.timestamp + 1199);

        bytes memory sig = _signClaim(swapId, PREIMAGE, uint64(block.timestamp + 1 days));
        vm.prank(receiver);
        htlc.claimWithSig(swapId, PREIMAGE, uint64(block.timestamp + 1 days), sig);

        assertEq(address(htlc).balance, 0);
        assertEq(sender.balance, 90 ether);
        assertEq(receiver.balance, 110 ether);

        (uint256 penalty,) = htlc.previewPenalty(swapId, uint64(block.timestamp));
        // View still works and reflects claim-time region; S1 expects zero-penalty zone.
        assertEq(penalty, 0);
    }

    function test_S2_ClaimInPenaltyWindow_LinearPenalty() public {
        bytes32 swapId = _openAndDeposit(5);
        vm.warp(block.timestamp + 1500);

        (uint256 previewPenaltyAmount, uint256 previewRefund) = htlc.previewPenalty(swapId, uint64(block.timestamp));
        assertEq(previewPenaltyAmount, 1 ether);
        assertEq(previewRefund, 1 ether);

        bytes memory sig = _signClaim(swapId, PREIMAGE, uint64(block.timestamp + 1 days));
        vm.prank(receiver);
        htlc.claimWithSig(swapId, PREIMAGE, uint64(block.timestamp + 1 days), sig);

        assertEq(sender.balance, 91 ether);
        assertEq(receiver.balance, 109 ether);
        assertEq(address(htlc).balance, 0);
    }

    function test_S3_AfterExpiry_FullDepositSlashedOnRefund() public {
        bytes32 swapId = _openAndDeposit(7);
        vm.warp(block.timestamp + 1801);

        vm.prank(sender);
        htlc.refund(swapId);

        assertEq(sender.balance, 102 ether);
        assertEq(receiver.balance, 98 ether);
        assertEq(address(htlc).balance, 0);
    }

    function test_S4_NoDepositAfter600s_RefundWithoutPenalty() public {
        bytes32 swapId = _openSwapOnly(9);
        vm.warp(block.timestamp + 601);

        vm.prank(sender);
        htlc.refund(swapId);

        assertEq(sender.balance, 100 ether);
        assertEq(receiver.balance, 100 ether);
        assertEq(address(htlc).balance, 0);
    }

    function test_S6_FrontRunningAttempt_CannotStealFunds() public {
        bytes32 swapId = _openAndDeposit(11);
        vm.warp(block.timestamp + 100);

        bytes memory sig = _signClaim(swapId, PREIMAGE, uint64(block.timestamp + 1 days));

        // Attacker copies calldata and broadcasts first. The contract still pays the designated receiver.
        vm.prank(attacker);
        htlc.claimWithSig(swapId, PREIMAGE, uint64(block.timestamp + 1 days), sig);

        assertEq(sender.balance, 90 ether);
        assertEq(receiver.balance, 110 ether);
        assertEq(address(htlc).balance, 0);
    }

    function test_S7_AggregatedClaimGas_RemainsSublinearForN20_50_100() public {
        uint256 gas20 = _measureAggregatedClaimGas(20, 101);
        uint256 gas50 = _measureAggregatedClaimGas(50, 102);
        uint256 gas100 = _measureAggregatedClaimGas(100, 103);

        emit log_named_uint("aggregated claim gas N=20", gas20);
        emit log_named_uint("aggregated claim gas N=50", gas50);
        emit log_named_uint("aggregated claim gas N=100", gas100);

        assertLt(gas20, 250_000);
        assertLt(gas50, 250_000);
        assertLt(gas100, 250_000);
    }

    function test_S7_CommitteeVerificationUpperBoundGas_N20_50_100() public {
        uint256 gas20 = _measureCommitteeVerifyGas(20);
        uint256 gas50 = _measureCommitteeVerifyGas(50);
        uint256 gas100 = _measureCommitteeVerifyGas(100);

        emit log_named_uint("committee verify gas N=20", gas20);
        emit log_named_uint("committee verify gas N=50", gas50);
        emit log_named_uint("committee verify gas N=100", gas100);

        // Conservative ceiling relative to typical L2 / testnet block gas envelopes.
        assertLt(gas100, 30_000_000);
        assertLt(gas50, gas100);
        assertLt(gas20, gas50);
    }

    function _openSwapOnly(uint16 participantCount) internal returns (bytes32 swapId) {
        swapId = keccak256(abi.encodePacked("swap", participantCount, block.timestamp, address(this)));
        vm.prank(sender);
        htlc.createSwap{value: AMOUNT}(
            swapId,
            receiver,
            authority,
            HASHLOCK,
            uint128(DEPOSIT),
            DEPOSIT_DELAY,
            TIMELOCK_DELAY,
            PENALTY_WINDOW,
            participantCount
        );
    }

    function _openAndDeposit(uint16 participantCount) internal returns (bytes32 swapId) {
        swapId = _openSwapOnly(participantCount);
        vm.prank(receiver);
        htlc.depositReceiver{value: DEPOSIT}(swapId);
    }

    function _signClaim(bytes32 swapId, bytes32 preimage, uint64 sigDeadline) internal view returns (bytes memory) {
        bytes32 digest = htlc.getClaimDigest(swapId, preimage, sigDeadline);
        (uint8 v, bytes32 r, bytes32 s) = vm.sign(authorityPk, digest);
        return abi.encodePacked(r, s, v);
    }

    function _measureAggregatedClaimGas(uint16 participantCount, uint256 seed) internal returns (uint256 used) {
        bytes32 swapId = _openAndDeposit(participantCount);
        vm.warp(block.timestamp + 1000 + seed);
        bytes memory sig = _signClaim(swapId, PREIMAGE, uint64(block.timestamp + 1 days));

        uint256 beforeGas = gasleft();
        vm.prank(receiver);
        htlc.claimWithSig(swapId, PREIMAGE, uint64(block.timestamp + 1 days), sig);
        used = beforeGas - gasleft();
    }

    function _measureCommitteeVerifyGas(uint256 n) internal returns (uint256 used) {
        bytes32 digest = keccak256(abi.encodePacked("bench", n));
        address[] memory committee = new address[](n);
        bytes[] memory signatures = new bytes[](n);

        for (uint256 i = 0; i < n; ++i) {
            uint256 pk = 1000 + i;
            committee[i] = vm.addr(pk);
            (uint8 v, bytes32 r, bytes32 s) = vm.sign(pk, digest);
            signatures[i] = abi.encodePacked(r, s, v);
        }

        uint256 beforeGas = gasleft();
        uint256 validCount = htlc.benchmarkVerifyCommitteeSignatures(digest, committee, signatures);
        used = beforeGas - gasleft();
        assertEq(validCount, n);
    }

    function test_Integration_GoSignature_E2E() public {
        address mpcAuthority = 0xeD06e969EfdeC2790489f8580B3e1147be52667F;
        bytes32 swapId = keccak256("swap-001");
        uint64 fixedDeadline = 999999;

        // 1. TẠO SWAP VÀ NẠP CỌC TRƯỚC
        _createSwapForIntegration(receiver, mpcAuthority, swapId);

        vm.deal(receiver, DEPOSIT);
        vm.prank(receiver);
        htlc.depositReceiver{value: DEPOSIT}(swapId);

        // Tua thời gian đến vùng an toàn
        vm.warp(block.timestamp + 1000);

        // 2. BÂY GIỜ MỚI LẤY DIGEST (Swap đã tồn tại nên sẽ không bị lỗi)
        bytes32 realDigest = htlc.getClaimDigest(swapId, PREIMAGE, fixedDeadline);
        console.log("=== COPY DIGEST NAY CHO GO MPC ===");
        console.logBytes32(realDigest);
        console.log("==================================");

        // 3. Gọi hàm claim với chữ ký cũ (Sẽ bị báo InvalidSignature ở bước này nhưng log ở trên đã được in ra)
        bytes memory sig = hex"6211a61a2b8c83fb427017bc764b49c981902abcfa9f6e8c26b5487337a6116704abf96e9915e37965c1a6fdb9cc635679c055ca6fb3b9be97cbd3c11a9040c200";
        
        if (uint8(sig[64]) < 27) {
            sig[64] = bytes1(uint8(sig[64]) + 27);
        }

        vm.prank(receiver);
        htlc.claimWithSig(swapId, PREIMAGE, fixedDeadline, sig);

        assertEq(address(htlc).balance, 0, "Swap should be completed");
        (uint256 penalty, ) = htlc.previewPenalty(swapId, uint64(block.timestamp));
        assertEq(penalty, 0, "Penalty must be zero");
    }

    function _createSwapForIntegration(address receiver_, address authority_, bytes32 swapId_) internal {
        vm.deal(sender, AMOUNT);
        vm.prank(sender);
        htlc.createSwap{value: AMOUNT}(
            swapId_,
            receiver_,
            authority_,
            HASHLOCK,
            uint128(DEPOSIT),
            DEPOSIT_DELAY,
            TIMELOCK_DELAY,
            PENALTY_WINDOW,
            3 // participantCount (ví dụ = 3)
        );
    }

    /*//////////////////////////////////////////////////////////////////////////
                                    HELPERS
    //////////////////////////////////////////////////////////////////////////*/

    function _splitSignature(bytes memory sig)
        internal
        pure
        returns (uint8 v, bytes32 r, bytes32 s)
    {
        require(sig.length == 65, "invalid signature length");
        assembly {
            r := mload(add(sig, 0x20))
            s := mload(add(sig, 0x40))
            v := byte(0, mload(add(sig, 0x60)))
        }
        if (v < 27) v += 27;
        require(v == 27 || v == 28, "invalid v");
    }

    function _ethSignedHash32(bytes32 innerHash) internal pure returns (bytes32) {
        return keccak256(abi.encodePacked("\x19Ethereum Signed Message:\n32", innerHash));
    }

    function _ethSignedHashBytes(bytes memory rawMessage) internal pure returns (bytes32) {
        return keccak256(
            abi.encodePacked(
                "\x19Ethereum Signed Message:\n",
                _uintToString(rawMessage.length),
                rawMessage
            )
        );
    }

    function _uintToString(uint256 value) internal pure returns (string memory) {
        if (value == 0) return "0";
        uint256 temp = value;
        uint256 digits;
        while (temp != 0) {
            digits++;
            temp /= 10;
        }
        bytes memory buffer = new bytes(digits);
        while (value != 0) {
            digits -= 1;
            buffer[digits] = bytes1(uint8(48 + uint256(value % 10)));
            value /= 10;
        }
        return string(buffer);
    }


}
