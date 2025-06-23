const { spawn } = require('child_process');

exports.handler = async (event, context) => {
  try {
    console.log('Manual task sync triggered');

    // Execute the Go CLI tool for task sync
    const result = await executeGoCommand('sync-tasks');

    if (result.success) {
      return {
        statusCode: 200,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          success: true,
          message: 'Task synchronization completed successfully',
          output: result.output
        })
      };
    } else {
      return {
        statusCode: 500,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          success: false,
          error: result.error
        })
      };
    }

  } catch (error) {
    console.error('Error in sync-tasks function:', error);
    return {
      statusCode: 500,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        success: false,
        error: error.message
      })
    };
  }
};

async function executeGoCommand(command, args = [], envVars = {}) {
  return new Promise((resolve) => {
    const env = { ...process.env, ...envVars };
    const child = spawn('./bin/observe-yor-estimates', [command, ...args], { 
      env,
      cwd: process.cwd()
    });

    let stdout = '';
    let stderr = '';

    child.stdout.on('data', (data) => {
      stdout += data.toString();
    });

    child.stderr.on('data', (data) => {
      stderr += data.toString();
    });

    child.on('close', (code) => {
      if (code === 0) {
        resolve({ success: true, output: stdout });
      } else {
        resolve({ success: false, error: stderr || `Exit code: ${code}` });
      }
    });

    child.on('error', (error) => {
      resolve({ success: false, error: error.message });
    });
  });
} 