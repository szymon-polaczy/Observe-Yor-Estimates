#!/usr/bin/env node

// Test script to check binary path resolution
const path = require('path');
const fs = require('fs');

console.log('Testing binary path resolution...');
console.log('Current working directory:', process.cwd());
console.log('__dirname:', __dirname);

// Simulate the paths from the Netlify function
const possiblePaths = [
  path.join(__dirname, 'observe-yor-estimates'),           // Same directory as test script
  path.join(__dirname, '..', 'observe-yor-estimates'),     // Original path
  path.join(process.cwd(), 'observe-yor-estimates'),       // Working directory
  './observe-yor-estimates',                               // Relative to cwd
  'observe-yor-estimates',                                 // Just the binary name
  '/var/task/observe-yor-estimates',                       // Lambda task root
  path.join(process.env.LAMBDA_TASK_ROOT || '/var/task', 'observe-yor-estimates') // Using env var
];

console.log('\nChecking paths:');
for (const possiblePath of possiblePaths) {
  try {
    const resolvedPath = path.resolve(possiblePath);
    if (fs.existsSync(possiblePath)) {
      const stats = fs.statSync(possiblePath);
      console.log(`✓ ${possiblePath} -> ${resolvedPath} (size: ${stats.size}, executable: ${(stats.mode & parseInt('111', 8)) !== 0})`);
    } else {
      console.log(`✗ ${possiblePath} -> ${resolvedPath} (NOT FOUND)`);
    }
  } catch (error) {
    console.log(`✗ ${possiblePath} (ERROR: ${error.message})`);
  }
}

// Check if binary works
const binaryPath = path.join(__dirname, 'observe-yor-estimates');
if (fs.existsSync(binaryPath)) {
  console.log('\nTesting binary execution...');
  const { spawn } = require('child_process');
  
  const child = spawn(binaryPath, ['--help'], { stdio: 'pipe' });
  
  child.stdout.on('data', (data) => {
    console.log('stdout:', data.toString());
  });
  
  child.stderr.on('data', (data) => {
    console.log('stderr:', data.toString());
  });
  
  child.on('close', (code) => {
    console.log(`Binary test completed with exit code: ${code}`);
  });
  
  child.on('error', (error) => {
    console.log(`Binary test failed: ${error.message}`);
  });
} else {
  console.log('\nBinary not found for testing');
}
