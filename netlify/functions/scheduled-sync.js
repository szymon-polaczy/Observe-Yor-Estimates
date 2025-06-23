const { spawn } = require('child_process');

exports.handler = async (event, context) => {
  try {
    const { type = 'task-sync' } = event.queryStringParameters || {};
    
    console.log(`Scheduled sync triggered: ${type}`);

    let command, args = [];
    switch (type) {
      case 'task-sync':
        command = 'sync-tasks';
        break;
      case 'time-entries-sync':
        command = 'sync-time-entries';
        break;
      case 'daily-update':
        command = 'update';
        args = ['daily'];
        break;
      case 'weekly-update':
        command = 'update';
        args = ['weekly'];
        break;
      case 'monthly-update':
        command = 'update';
        args = ['monthly'];
        break;
      default:
        return {
          statusCode: 400,
          body: JSON.stringify({ error: `Unknown sync type: ${type}` })
        };
    }

    // Execute the Go CLI tool
    const result = await executeGoCommand(command, args);

    if (result.success) {
      console.log(`Scheduled ${type} completed successfully`);
      return {
        statusCode: 200,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          success: true,
          type,
          message: `${type} completed successfully`,
          output: result.output
        })
      };
    } else {
      console.error(`Scheduled ${type} failed:`, result.error);
      return {
        statusCode: 500,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          success: false,
          type,
          error: result.error
        })
      };
    }

  } catch (error) {
    console.error('Error in scheduled-sync function:', error);
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