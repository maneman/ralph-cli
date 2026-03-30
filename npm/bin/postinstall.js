#!/usr/bin/env node

// Verify ralph binary is available after install
// This runs as postinstall to give early feedback

const { execFileSync } = require('child_process');

try {
  // Try to find the binary (same logic as ralph.js)
  const os = require('os');
  const platform = os.platform();
  const arch = os.arch();
  const key = `${platform}-${arch}`;
  const pkg = `@ralph-cli/${key}`;
  
  try {
    require.resolve(`${pkg}/ralph`);
    console.log(`ralph-cli: Binary found via ${pkg}`);
  } catch (e) {
    // Try PATH
    try {
      execFileSync('ralph', ['--version'], { stdio: 'pipe' });
      console.log('ralph-cli: Binary found in PATH');
    } catch (e2) {
      console.warn(
        'ralph-cli: Warning - ralph binary not found.\n' +
        'Install via: go install github.com/mane/ralph-cli/cmd/ralph@latest\n' +
        'Or set RALPH_BINARY_PATH environment variable.'
      );
    }
  }
} catch (err) {
  // Don't fail install on postinstall errors
}
