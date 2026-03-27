// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

/// @title MP-HTLC-LGP (On-chain layer)
/// @notice Implements a sender-funded HTLC with receiver collateral and linear griefing penalty.
/// @dev Production claim path verifies a single aggregated signature (e.g. TSS output) via ecrecover.
///      A committee verification helper is included only for gas-stress research, not for production use.
contract MPHTLCLGP {
    uint256 internal constant SECP256K1N_DIV_2 =
        0x7fffffffffffffffffffffffffffffff5d576e7357a4501ddfe92f46681b20a0;

    error InvalidParameters();
    error SwapAlreadyExists();
    error SwapNotFound();
    error NotSender();
    error DepositWindowClosed();
    error DepositWindowNotPassed();
    error WrongReceiver();
    error WrongDepositAmount();
    error AlreadyDeposited();
    error AlreadyFinalized();
    error DepositMissing();
    error TooEarly();
    error SignatureExpired();
    error InvalidPreimage();
    error InvalidSignature();
    error TransferFailed();
    error ArrayLengthMismatch();

    struct Swap {
        address sender;
        address receiver;
        address claimAuthority; // Aggregated/TSS public key mapped to EOA-style address for ecrecover
        uint128 amount;
        uint128 depositRequired;
        uint64 createdAt;
        uint64 depositDeadline;
        uint64 timelock;
        uint64 penaltyWindow;
        uint16 participantCount; // research metadata only; production gas stays O(1)
        bool receiverDeposited;
        bool claimed;
        bool refunded;
        bytes32 hashlock;
    }

    struct SwapView {
        address sender;
        address receiver;
        address claimAuthority;
        uint256 amount;
        uint256 depositRequired;
        uint256 createdAt;
        uint256 depositDeadline;
        uint256 timelock;
        uint256 penaltyWindow;
        uint256 participantCount;
        bool receiverDeposited;
        bool claimed;
        bool refunded;
        bytes32 hashlock;
    }

    mapping(bytes32 => Swap) internal swaps;

    event SwapCreated(
        bytes32 indexed swapId,
        address indexed sender,
        address indexed receiver,
        address claimAuthority,
        uint256 amount,
        uint256 depositRequired,
        uint256 depositDeadline,
        uint256 timelock,
        uint256 penaltyWindow,
        uint256 participantCount,
        bytes32 hashlock
    );

    event ReceiverDeposited(bytes32 indexed swapId, address indexed receiver, uint256 amount);

    event Claimed(
        bytes32 indexed swapId,
        address indexed caller,
        address indexed receiver,
        bytes32 preimage,
        uint256 penaltyPaidToSender,
        uint256 depositRefundedToReceiver
    );

    event Refunded(
        bytes32 indexed swapId,
        address indexed sender,
        uint256 senderRefundAmount,
        uint256 penaltyApplied
    );

    event CommitteeVerificationBenchmarked(bytes32 indexed digest, uint256 validCount);

    function createSwap(
        bytes32 swapId,
        address receiver,
        address claimAuthority,
        bytes32 hashlock,
        uint128 depositRequired,
        uint64 depositDelay,
        uint64 timelockDelay,
        uint64 penaltyWindow,
        uint16 participantCount
    ) external payable {
        if (
            swapId == bytes32(0) ||
            receiver == address(0) ||
            claimAuthority == address(0) ||
            hashlock == bytes32(0) ||
            msg.value == 0 ||
            timelockDelay == 0 ||
            penaltyWindow == 0 ||
            timelockDelay <= penaltyWindow ||
            depositDelay >= timelockDelay
        ) revert InvalidParameters();

        Swap storage s = swaps[swapId];
        if (s.sender != address(0)) revert SwapAlreadyExists();

        uint64 createdAt = uint64(block.timestamp);
        swaps[swapId] = Swap({
            sender: msg.sender,
            receiver: receiver,
            claimAuthority: claimAuthority,
            amount: uint128(msg.value),
            depositRequired: depositRequired,
            createdAt: createdAt,
            depositDeadline: createdAt + depositDelay,
            timelock: createdAt + timelockDelay,
            penaltyWindow: penaltyWindow,
            participantCount: participantCount,
            receiverDeposited: false,
            claimed: false,
            refunded: false,
            hashlock: hashlock
        });

        emit SwapCreated(
            swapId,
            msg.sender,
            receiver,
            claimAuthority,
            msg.value,
            depositRequired,
            createdAt + depositDelay,
            createdAt + timelockDelay,
            penaltyWindow,
            participantCount,
            hashlock
        );
    }

    function depositReceiver(bytes32 swapId) external payable {
        Swap storage s = _mustGetSwap(swapId);
        if (s.claimed || s.refunded) revert AlreadyFinalized();
        if (msg.sender != s.receiver) revert WrongReceiver();
        if (s.receiverDeposited) revert AlreadyDeposited();
        if (block.timestamp > s.depositDeadline) revert DepositWindowClosed();
        if (msg.value != s.depositRequired) revert WrongDepositAmount();

        s.receiverDeposited = true;
        emit ReceiverDeposited(swapId, msg.sender, msg.value);
    }

    /// @notice Claim locked amount using a single aggregated signature produced off-chain.
    /// @dev Anyone may relay the transaction, but funds always go to the designated receiver.
    function claimWithSig(bytes32 swapId, bytes32 preimage, uint64 sigDeadline, bytes calldata signature) external {
        Swap storage s = _mustGetSwap(swapId);
        if (s.claimed || s.refunded) revert AlreadyFinalized();
        if (!s.receiverDeposited) revert DepositMissing();
        if (block.timestamp > s.timelock) revert TooEarly(); // use refund path after expiry
        if (block.timestamp > sigDeadline) revert SignatureExpired();
        if (keccak256(abi.encodePacked(preimage)) != s.hashlock) revert InvalidPreimage();

        bytes32 digest = getClaimDigest(swapId, preimage, sigDeadline);
        address recovered = _recoverSigner(digest, signature);
        if (recovered != s.claimAuthority) revert InvalidSignature();

        (uint256 penaltyAmount, uint256 depositRefund) = previewPenalty(swapId, uint64(block.timestamp));
        s.claimed = true;

        uint256 amountToReceiver = uint256(s.amount) + depositRefund;
        uint256 amountToSender = penaltyAmount;

        if (amountToReceiver != 0) _safeTransferETH(s.receiver, amountToReceiver);
        if (amountToSender != 0) _safeTransferETH(s.sender, amountToSender);

        emit Claimed(swapId, msg.sender, s.receiver, preimage, penaltyAmount, depositRefund);
    }

    /// @notice Refund the sender.
    /// @dev Case A: receiver never deposited and deposit deadline passed => sender gets principal back, penalty 0.
    ///      Case B: receiver deposited but timelock passed without claim => sender gets principal + 100% deposit.
    function refund(bytes32 swapId) external {
        Swap storage s = _mustGetSwap(swapId);
        if (s.claimed || s.refunded) revert AlreadyFinalized();
        if (msg.sender != s.sender) revert NotSender();

        uint256 refundAmount;
        uint256 penaltyApplied;

        if (!s.receiverDeposited) {
            if (block.timestamp <= s.depositDeadline) revert DepositWindowNotPassed();
            refundAmount = uint256(s.amount);
            penaltyApplied = 0;
        } else {
            if (block.timestamp <= s.timelock) revert TooEarly();
            refundAmount = uint256(s.amount) + uint256(s.depositRequired);
            penaltyApplied = uint256(s.depositRequired);
        }

        s.refunded = true;
        _safeTransferETH(s.sender, refundAmount);
        emit Refunded(swapId, s.sender, refundAmount, penaltyApplied);
    }

    /// @notice Research-only helper to benchmark the gas of verifying N signatures on-chain.
    /// @dev This is intentionally not used by the production claim path, which is O(1).
    function benchmarkVerifyCommitteeSignatures(
        bytes32 digest,
        address[] calldata committee,
        bytes[] calldata signatures
    ) external returns (uint256 validCount) {
        uint256 len = committee.length;
        if (len != signatures.length) revert ArrayLengthMismatch();

        for (uint256 i = 0; i < len; ++i) {
            if (_recoverSigner(digest, signatures[i]) == committee[i]) {
                unchecked {
                    ++validCount;
                }
            }
        }

        emit CommitteeVerificationBenchmarked(digest, validCount);
    }

    function previewPenalty(bytes32 swapId, uint64 claimTime)
        public
        view
        returns (uint256 penaltyAmount, uint256 depositRefund)
    {
        Swap storage s = _mustGetSwap(swapId);
        if (!s.receiverDeposited || s.depositRequired == 0) {
            return (0, s.receiverDeposited ? 0 : 0);
        }

        uint256 penaltyStart = uint256(s.timelock) - uint256(s.penaltyWindow);

        if (claimTime <= penaltyStart) {
            return (0, uint256(s.depositRequired));
        }

        if (claimTime >= s.timelock) {
            return (uint256(s.depositRequired), 0);
        }

        uint256 elapsed = uint256(claimTime) - penaltyStart;
        penaltyAmount = (uint256(s.depositRequired) * elapsed) / uint256(s.penaltyWindow);
        if (penaltyAmount > s.depositRequired) {
            penaltyAmount = s.depositRequired;
        }
        depositRefund = uint256(s.depositRequired) - penaltyAmount;
    }

    function getClaimDigest(bytes32 swapId, bytes32 preimage, uint64 sigDeadline) public view returns (bytes32) {
        Swap storage s = _mustGetSwap(swapId);
        bytes32 structHash = keccak256(
            abi.encode(
                keccak256(
                    "Claim(bytes32 swapId,address receiver,bytes32 preimage,uint64 sigDeadline,address verifyingContract,uint256 chainId)"
                ),
                swapId,
                s.receiver,
                preimage,
                sigDeadline,
                address(this),
                block.chainid
            )
        );
        return keccak256(abi.encodePacked("\x19Ethereum Signed Message:\n32", structHash));
    }

    function getSwap(bytes32 swapId) external view returns (SwapView memory v) {
        Swap storage s = _mustGetSwap(swapId);
        v = SwapView({
            sender: s.sender,
            receiver: s.receiver,
            claimAuthority: s.claimAuthority,
            amount: s.amount,
            depositRequired: s.depositRequired,
            createdAt: s.createdAt,
            depositDeadline: s.depositDeadline,
            timelock: s.timelock,
            penaltyWindow: s.penaltyWindow,
            participantCount: s.participantCount,
            receiverDeposited: s.receiverDeposited,
            claimed: s.claimed,
            refunded: s.refunded,
            hashlock: s.hashlock
        });
    }

    function _mustGetSwap(bytes32 swapId) internal view returns (Swap storage s) {
        s = swaps[swapId];
        if (s.sender == address(0)) revert SwapNotFound();
    }

    function _safeTransferETH(address to, uint256 value) internal {
        (bool ok,) = payable(to).call{value: value}("");
        if (!ok) revert TransferFailed();
    }

    function _recoverSigner(bytes32 digest, bytes calldata signature) internal pure returns (address) {
        if (signature.length != 65) revert InvalidSignature();

        bytes32 r;
        bytes32 s;
        uint8 v;
        assembly {
            r := calldataload(signature.offset)
            s := calldataload(add(signature.offset, 0x20))
            v := byte(0, calldataload(add(signature.offset, 0x40)))
        }

        if (uint256(s) > SECP256K1N_DIV_2) revert InvalidSignature();
        if (v != 27 && v != 28) revert InvalidSignature();

        address signer = ecrecover(digest, v, r, s);
        if (signer == address(0)) revert InvalidSignature();
        return signer;
    }

    receive() external payable {}
}
