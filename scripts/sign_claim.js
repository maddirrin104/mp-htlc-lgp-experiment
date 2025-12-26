import 'dotenv/config';
import { ethers } from 'ethers';

// Usage:
// node scripts/sign_claim.js --lockId 0x... --receiver 0x... --chainId 11155111 --contract 0x...

function arg(name) {
  const i = process.argv.indexOf(`--${name}`);
  if (i === -1 || i + 1 >= process.argv.length) return null;
  return process.argv[i + 1];
}

const lockId = arg('lockId');
const receiver = arg('receiver');
const chainId = BigInt(arg('chainId'));
const contract = arg('contract');

if (!lockId || !receiver || !chainId || !contract) {
  console.error('Missing args. Need --lockId --receiver --chainId --contract');
  process.exit(1);
}

const pk = process.env.SIGNER_PK;
if (!pk) {
  console.error('Missing SIGNER_PK in .env');
  process.exit(1);
}

const wallet = new ethers.Wallet(pk);

const domain = {
  name: 'MPHTLC_LGP',
  version: '1',
  chainId: Number(chainId),
  verifyingContract: contract,
};

const types = {
  Claim: [
    { name: 'lockId', type: 'bytes32' },
    { name: 'receiver', type: 'address' },
  ],
};

const value = { lockId, receiver };

const sig = await wallet.signTypedData(domain, types, value);
console.log(sig);
