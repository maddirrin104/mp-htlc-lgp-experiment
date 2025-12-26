// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {IERC20} from "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import {SafeERC20} from "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";
import {EIP712} from "@openzeppelin/contracts/utils/cryptography/EIP712.sol";
import {ECDSA} from "@openzeppelin/contracts/utils/cryptography/ECDSA.sol";

/// @notice MP-HTLC-LGP: HTLC + Linear Griefing Penalty + off-chain signer (TSS) authorization.
/// @dev Testnet-oriented implementation for experiments (S1-S4 + extensions).
contract MPHTLC_LGP is EIP712 {
    using SafeERC20 for IERC20;

    bytes32 public constant CLAIM_TYPEHASH = keccak256("Claim(bytes32 lockId,address receiver)");

    struct Lock {
        address token;
        address sender;
        address receiver;
        address signer; // ADDR_TSS (aggregate public key address) used to authorize claim
        uint256 amount;
        bytes32 hashlock;
        uint256 timelock;        // absolute timestamp T
        uint256 penaltyWindow;   // Tw (seconds)
        uint256 depositRequired; // wei
        uint256 depositWindow;   // seconds since createdAt
        uint256 createdAt;
        bool depositConfirmed;
        bool claimed;
        bool refunded;
    }

    mapping(bytes32 => Lock) public locks;

    event Locked(bytes32 indexed lockId, address indexed sender, address indexed receiver, address token, uint256 amount, bytes32 hashlock);
    event ParticipationConfirmed(bytes32 indexed lockId, address indexed receiver, uint256 deposit);
    event Claimed(bytes32 indexed lockId, address indexed receiver, bytes32 preimage, uint256 penalty, uint256 depositRefund);
    event Refunded(bytes32 indexed lockId, address indexed to, uint256 tokenAmount, uint256 depositPaid);

    error LockExists();
    error LockNotFound();
    error NotReceiver();
    error NotSender();
    error BadPreimage();
    error BadSignature();
    error DepositNotConfirmed();
    error TooLate();
    error TooEarly();
    error AlreadyFinalized();
    error BadDeposit();

    constructor() EIP712("MPHTLC_LGP", "1") {}

    /// @notice Create a lock and transfer token into contract.
    /// @param lockId unique id (recommended: keccak256 of all params incl. nonce)
    function lock(
        bytes32 lockId,
        address token,
        address receiver,
        address signer,
        uint256 amount,
        bytes32 hashlock,
        uint256 timelock,
        uint256 penaltyWindow,
        uint256 depositRequired,
        uint256 depositWindow
    ) external {
        if (locks[lockId].createdAt != 0) revert LockExists();
        require(receiver != address(0) && signer != address(0), "bad addr");
        require(timelock > block.timestamp, "timelock in past");
        require(penaltyWindow <= timelock, "bad window");

        IERC20(token).safeTransferFrom(msg.sender, address(this), amount);

        locks[lockId] = Lock({
            token: token,
            sender: msg.sender,
            receiver: receiver,
            signer: signer,
            amount: amount,
            hashlock: hashlock,
            timelock: timelock,
            penaltyWindow: penaltyWindow,
            depositRequired: depositRequired,
            depositWindow: depositWindow,
            createdAt: block.timestamp,
            depositConfirmed: false,
            claimed: false,
            refunded: false
        });

        emit Locked(lockId, msg.sender, receiver, token, amount, hashlock);
    }

    /// @notice Receiver deposits ETH to participate.
    function confirmParticipation(bytes32 lockId) external payable {
        Lock storage L = locks[lockId];
        if (L.createdAt == 0) revert LockNotFound();
        if (L.refunded || L.claimed) revert AlreadyFinalized();
        if (msg.sender != L.receiver) revert NotReceiver();

        if (block.timestamp > L.createdAt + L.depositWindow) revert TooLate();
        if (msg.value != L.depositRequired) revert BadDeposit();

        L.depositConfirmed = true;
        emit ParticipationConfirmed(lockId, msg.sender, msg.value);
    }

    /// @notice Receiver claims token by revealing preimage + providing signer authorization.
    function claimWithSig(bytes32 lockId, bytes32 preimage, bytes calldata sig) external {
        Lock storage L = locks[lockId];
        if (L.createdAt == 0) revert LockNotFound();
        if (L.refunded || L.claimed) revert AlreadyFinalized();
        if (msg.sender != L.receiver) revert NotReceiver();
        if (!L.depositConfirmed) revert DepositNotConfirmed();

        if (keccak256(abi.encodePacked(preimage)) != L.hashlock) revert BadPreimage();
        if (block.timestamp >= L.timelock) revert TooLate();

        // EIP-712 verify: signature must bind lockId and receiver.
        bytes32 structHash = keccak256(abi.encode(CLAIM_TYPEHASH, lockId, L.receiver));
        bytes32 digest = _hashTypedDataV4(structHash);
        address recovered = ECDSA.recover(digest, sig);
        if (recovered != L.signer) revert BadSignature();

        (uint256 penalty, uint256 refundDeposit) = _calcPenalty(L, block.timestamp);

        L.claimed = true;

        // 1) Transfer token to receiver
        IERC20(L.token).safeTransfer(L.receiver, L.amount);

        // 2) Pay deposit refund to receiver
        if (refundDeposit > 0) {
            (bool ok1, ) = payable(L.receiver).call{value: refundDeposit}("");
            require(ok1, "refund fail");
        }

        // 3) Pay penalty to sender
        if (penalty > 0) {
            (bool ok2, ) = payable(L.sender).call{value: penalty}("");
            require(ok2, "penalty pay fail");
        }

        emit Claimed(lockId, L.receiver, preimage, penalty, refundDeposit);
    }

    /// @notice Sender refunds token (and possibly gets deposit) after timeouts.
    function refund(bytes32 lockId) external {
        Lock storage L = locks[lockId];
        if (L.createdAt == 0) revert LockNotFound();
        if (L.refunded || L.claimed) revert AlreadyFinalized();
        if (msg.sender != L.sender) revert NotSender();

        // Case A: no deposit, depositWindow expired
        if (!L.depositConfirmed) {
            if (block.timestamp < L.createdAt + L.depositWindow) revert TooEarly();
            L.refunded = true;
            IERC20(L.token).safeTransfer(L.sender, L.amount);
            emit Refunded(lockId, L.sender, L.amount, 0);
            return;
        }

        // Case B: deposit confirmed, but no claim until timelock
        if (block.timestamp < L.timelock) revert TooEarly();
        L.refunded = true;
        IERC20(L.token).safeTransfer(L.sender, L.amount);
        (bool ok, ) = payable(L.sender).call{value: L.depositRequired}("");
        require(ok, "deposit to sender fail");
        emit Refunded(lockId, L.sender, L.amount, L.depositRequired);
    }

    function _calcPenalty(Lock storage L, uint256 claimTime) internal view returns (uint256 penalty, uint256 refundDeposit) {
        // Penalty starts at timelock - penaltyWindow
        uint256 penaltyStart = L.timelock - L.penaltyWindow;

        if (claimTime <= penaltyStart) {
            return (0, L.depositRequired);
        }

        if (claimTime >= L.timelock) {
            return (L.depositRequired, 0);
        }

        uint256 elapsed = claimTime - penaltyStart; // in [1..penaltyWindow-1]
        // Linear penalty: depositRequired * elapsed / penaltyWindow
        uint256 p = (L.depositRequired * elapsed) / L.penaltyWindow;
        if (p > L.depositRequired) p = L.depositRequired;
        return (p, L.depositRequired - p);
    }

    receive() external payable {}
}
