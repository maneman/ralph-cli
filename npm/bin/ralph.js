#!/usr/bin/env node

// Find the platform-specific binary
// Check: 1) platform package, 2) RALPH_BINARY_PATH env var, 3) PATH

const { execFileSync } = require('child_process');
const path = require('path');
const os = require('os');

function getPlatformPackage() {
  const platform = os.platform();
  const arch = os.arch();
  
  const platformMap = {
    'darwin-arm64': '@ralph-cli/darwin-arm64',
    'darwin-x64': '@ralph-cli/darwin-x64',
    'linux-arm64': '@ralph-cli/linux-arm64',
    'linux-x64': '@ralph-cli/linux-x64',
  };
  
  const key = `${platform}-${arch}`;
  const pkg = platformMap[key];
  
  if (!pkg) {
    throw new Error(`Unsupported platform: ${key}. Ralph CLI supports: ${Object.keys(platformMap).join(', ')}`);
  }
  
  return pkg;
}

function findBinary() {
  // 1. Check env var override
  if (process.env.RALPH_BINARY_PATH) {
    return process.env.RALPH_BINARY_PATH;
  }
  
  // 2. Try platform-specific npm package
  try {
    const pkg = getPlatformPackage();
    return require.resolve(`${pkg}/ralph`);
  } catch (e) {
    // Not installed via npm
  }
  
  // 3. Fall back to PATH
  return 'ralph';
}

try {
  const binary = findBinary();
  const args = process.argv.slice(2);
  
  const result = execFileSync(binary, args, {
    stdio: 'inherit',
    env: process.env,
  });
  
  process.exit(0);
} catch (err) {
  if (err.status !== undefined) {
    process.exit(err.status);
  }
  console.error('Failed to run ralph:', err.message);
  process.exit(1);
}
