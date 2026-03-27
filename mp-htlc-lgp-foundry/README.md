# MP-HTLC-LGP Foundry package

## Scope
This package implements the **on-chain** layer for MP-HTLC-LGP:
- sender-funded lock
- receiver collateral
- linear griefing penalty
- aggregated-signature claim path (`claimWithSig`)
- refund logic for no-deposit and timeout cases
- gas benchmark helper for N-party committee verification

## Contract model
### Time parameters
Recommended experimental profile used by the tests:
- deposit deadline: `600s`
- penalty window opens at `1200s`
- final timelock: `1800s`
- penalty window length: `600s`

This means:
- `t <= 1200s`: claim succeeds, `penalty = 0`
- `1200s < t < 1800s`: linear penalty, from `0%` to `100%` of receiver deposit
- `t > 1800s`: sender can refund principal and slash `100%` of receiver deposit
- `t > 600s` without receiver deposit: sender can refund principal, penalty `= 0`

### Front-running resistance
`claimWithSig` is safe against calldata-copy front-running because:
1. the signature binds `(swapId, receiver, preimage, deadline, contract, chainId)`
2. the payout is always sent to the pre-registered `receiver`
3. a copied transaction cannot redirect value to the attacker

## Setup
```bash
forge init mp-htlc-lgp-foundry
cd mp-htlc-lgp-foundry
forge install foundry-rs/forge-std
cp .env.example .env
```

Or just copy this repository layout into your Foundry workspace.

## Test
```bash
forge test -vvv
```

## Deploy
Load env vars first:
```bash
source .env
```

### Ethereum Sepolia
```bash
forge script script/DeployMPHTLCLGP.s.sol:DeployMPHTLCLGP \
  --rpc-url $SEPOLIA_RPC_URL \
  --broadcast \
  --private-key $PRIVATE_KEY
```

### Arbitrum Sepolia
```bash
forge script script/DeployMPHTLCLGP.s.sol:DeployMPHTLCLGP \
  --rpc-url $ARB_SEPOLIA_RPC_URL \
  --broadcast \
  --private-key $PRIVATE_KEY
```

### Optional verification
```bash
forge verify-contract <DEPLOYED_ADDRESS> src/MPHTLCLGP.sol:MPHTLCLGP \
  --chain sepolia \
  --etherscan-api-key $ETHERSCAN_API_KEY
```

For Arbitrum Sepolia, switch the `--chain` argument to the chain supported by your chosen explorer.

## Test coverage map
- `test_S1_ClaimEarly_NoPenalty`
- `test_S2_ClaimInPenaltyWindow_LinearPenalty`
- `test_S3_AfterExpiry_FullDepositSlashedOnRefund`
- `test_S4_NoDepositAfter600s_RefundWithoutPenalty`
- `test_S6_FrontRunningAttempt_CannotStealFunds`
- `test_S7_AggregatedClaimGas_RemainsSublinearForN20_50_100`
- `test_S7_CommitteeVerificationUpperBoundGas_N20_50_100`

## Notes for the next phase
This package assumes the off-chain MPC/TSS service outputs one final ECDSA-compatible signature addressable by `claimAuthority`. In the Go phase, that service will own:
- peer discovery
- threshold key generation
- distributed signing
- pre-claim timing checks and penalty warnings
