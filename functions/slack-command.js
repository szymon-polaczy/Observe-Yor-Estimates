// Netlify Function for Slack Commands
const { spawn } = require('child_process');
const { promisify } = require('util');
const querystring = require('querystring');
const https = require('https');
const http = require('http');

exports.handler = async (event, context) => {
  // Only allow POST requests
  if (event.httpMethod !== 'POST') {
    return {
      statusCode: 405,
      body: JSON.stringify({ error: 'Method not allowed' }),
    };
  }

  try {
    // Parse the command from the URL path
    const command = event.path.split('/').pop(); // Gets 'daily-update', 'weekly-update', etc.
    const validCommands = ['daily-update', 'weekly-update', 'monthly-update', 'full-sync'];
    
    if (!validCommands.includes(command)) {
      return {
        statusCode: 400,
        body: JSON.stringify({ error: 'Invalid command' }),
      };
    }

    // Parse the Slack request body
    const slackRequest = querystring.parse(event.body);
    const responseUrl = slackRequest.response_url;

    if (!responseUrl) {
      return {
        statusCode: 400,
        body: JSON.stringify({ error: 'Missing response_url' }),
      };
    }

    // Verify Slack request if token is configured
    const expectedToken = process.env.SLACK_VERIFICATION_TOKEN;
    if (expectedToken && slackRequest.token !== expectedToken) {
      return {
        statusCode: 401,
        body: JSON.stringify({ error: 'Invalid verification token' }),
      };
    }

    // Process the command synchronously and return the actual result
    return await processSlackCommand(command, responseUrl);

  } catch (error) {
    console.error('Error processing Slack command:', error);
    return {
      statusCode: 500,
      body: JSON.stringify({ error: 'Internal server error' }),
    };
  }
};

// Process the Slack command synchronously and return the result
async function processSlackCommand(command, responseUrl) {
  return new Promise((resolve, reject) => {
    try {
      console.log(`Processing ${command} command...`);
      
      // Execute the Go binary with the appropriate command
      // Pass --output-json flag to get JSON response instead of sending via response URL
      
      // Use absolute path for better reliability in Netlify environment
      const path = require('path');
      const fs = require('fs');
      
      // In Netlify, the binary should be in the project root directory
      // Netlify Functions run from the site root, so we need to go up from functions/ directory
      const possiblePaths = [
        path.resolve(__dirname, '..', 'observe-yor-estimates'), // Project root (most likely)
        path.resolve(process.cwd(), 'observe-yor-estimates'),   // Current working directory
        path.resolve(__dirname, 'observe-yor-estimates'),       // Functions directory (unlikely)
        './observe-yor-estimates',                              // Relative to cwd
        'observe-yor-estimates',                                // Just the binary name
        '/var/task/observe-yor-estimates',                      // Lambda task root (for AWS)
        path.join(process.env.LAMBDA_TASK_ROOT || '/var/task', 'observe-yor-estimates'), // Using env var
        path.join(process.env.NETLIFY_BUILD_BASE || '/opt/build/repo', 'observe-yor-estimates') // Netlify build base
      ];
      
      let goExecutable = null;
      console.log('Checking binary paths in Netlify environment...');
      
      for (const possiblePath of possiblePaths) {
        try {
          console.log(`Checking: ${possiblePath}`);
          if (fs.existsSync(possiblePath)) {
            const stats = fs.statSync(possiblePath);
            const isExecutable = (stats.mode & parseInt('111', 8)) !== 0;
            console.log(`✓ Found binary at: ${possiblePath} (size: ${stats.size}, executable: ${isExecutable})`);
            goExecutable = possiblePath;
            break;
          } else {
            console.log(`✗ Not found: ${possiblePath}`);
          }
        } catch (error) {
          console.log(`✗ Error checking ${possiblePath}: ${error.message}`);
        }
      }
      
      if (!goExecutable) {
        console.error('Binary not found in any expected location!');
        console.error('Environment information:');
        console.error(`- process.cwd(): ${process.cwd()}`);
        console.error(`- __dirname: ${__dirname}`);
        console.error(`- NETLIFY: ${process.env.NETLIFY}`);
        console.error(`- LAMBDA_TASK_ROOT: ${process.env.LAMBDA_TASK_ROOT}`);
        console.error(`- NETLIFY_BUILD_BASE: ${process.env.NETLIFY_BUILD_BASE}`);
        
        // List all files in the current directory and parent directory for debugging
        try {
          console.error('Files in current directory:');
          const currentFiles = fs.readdirSync(process.cwd());
          currentFiles.forEach(file => {
            const filePath = path.join(process.cwd(), file);
            const stats = fs.statSync(filePath);
            console.error(`  ${file} ${stats.isDirectory() ? '(dir)' : `(${stats.size} bytes)`}`);
          });
          
          console.error('Files in parent directory:');
          const parentDir = path.join(__dirname, '..');
          const parentFiles = fs.readdirSync(parentDir);
          parentFiles.forEach(file => {
            const filePath = path.join(parentDir, file);
            const stats = fs.statSync(filePath);
            console.error(`  ${file} ${stats.isDirectory() ? '(dir)' : `(${stats.size} bytes)`}`);
          });
        } catch (listError) {
          console.error('Error listing files:', listError.message);
        }
        
        // Use the most likely path for error reporting
        goExecutable = path.resolve(__dirname, '..', 'observe-yor-estimates');
      }
      
      console.log(`Attempting to spawn: ${goExecutable} with args: [${command}, --output-json]`);
      console.log(`Current working directory: ${process.cwd()}`);
      console.log(`__dirname: ${__dirname}`);
      console.log(`process.env.NETLIFY: ${process.env.NETLIFY}`);
      console.log(`process.env.LAMBDA_TASK_ROOT: ${process.env.LAMBDA_TASK_ROOT}`);
      
      // Log which path was selected
      console.log(`Selected executable path: ${goExecutable}`);
      console.log(`Available paths checked:`, possiblePaths);
      
      // Set up environment with proper database path for Netlify
      const envVars = { ...process.env };
      
      // Determine the appropriate database path for Netlify environment
      if (process.env.LAMBDA_TASK_ROOT) {
        // We're in a Lambda/Netlify function environment
        const dbPath = path.join(process.env.LAMBDA_TASK_ROOT, 'oye.db');
        envVars.DATABASE_PATH = dbPath;
        console.log(`Setting DATABASE_PATH to: ${dbPath}`);
        
        // Check if database exists, if not we may need to sync
        try {
          if (!fs.existsSync(dbPath)) {
            console.log(`Database file does not exist at: ${dbPath}`);
            console.log('This may be a first-time deployment or the database was not included in the deployment.');
          } else {
            const dbStats = fs.statSync(dbPath);
            console.log(`Database file exists: ${dbPath} (size: ${dbStats.size} bytes)`);
          }
        } catch (dbCheckError) {
          console.log(`Error checking database file: ${dbCheckError.message}`);
        }
      }
      
      const child = spawn(goExecutable, [command, '--output-json'], {
        stdio: ['pipe', 'pipe', 'pipe'],
        env: envVars,
        cwd: path.join(__dirname, '..')  // Ensure working directory is project root
      });

      let stdout = '';
      let stderr = '';

      // Add timeout to prevent hanging
      const timeout = setTimeout(() => {
        console.error(`Command ${command} timed out after 25 seconds`);
        child.kill('SIGTERM');
        resolve({
          statusCode: 200,
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({
            response_type: 'ephemeral',
            text: `❌ Error: ${command.replace('-', ' ')} command timed out`
          }),
        });
      }, 25000); // 25 second timeout

      child.stdout.on('data', (data) => {
        stdout += data.toString();
      });

      child.stderr.on('data', (data) => {
        stderr += data.toString();
      });

      child.on('close', async (code) => {
        clearTimeout(timeout); // Clear the timeout since process completed
        
        if (code === 0) {
          console.log(`Successfully executed ${command} command`);
          
          // Parse the stdout as JSON (Go binary should output the Slack message as JSON)
          try {
            const result = JSON.parse(stdout);
            resolve({
              statusCode: 200,
              headers: {
                'Content-Type': 'application/json',
              },
              body: JSON.stringify({
                response_type: 'in_channel',
                text: result.text,
                blocks: result.blocks
              }),
            });
          } catch (parseError) {
            console.error('Failed to parse Go binary output as JSON:', parseError);
            console.error('stdout:', stdout);
            resolve({
              statusCode: 200,
              headers: {
                'Content-Type': 'application/json',
              },
              body: JSON.stringify({
                response_type: 'ephemeral',
                text: `✅ ${command.replace('-', ' ')} completed successfully`
              }),
            });
          }
        } else {
          console.error(`Command ${command} failed with code ${code}`);
          console.error('stderr:', stderr);
          
          // Check if the error is database-related and provide helpful message
          let errorMessage = `❌ Error: ${command.replace('-', ' ')} command failed`;
          if (stderr.includes('Failed to initialize database') || stderr.includes('unable to open database file')) {
            errorMessage = `❌ Database Error: The database file is not accessible in the deployment environment. This indicates the database was not properly included in the Netlify deployment or needs to be initialized.`;
          } else if (stderr.includes('no such file or directory') && stderr.includes('.env')) {
            // This is just a warning about missing .env file, not a critical error
            errorMessage = `❌ Configuration Error: Environment variables are missing. Please check that TIMECAMP_API_KEY and SLACK_WEBHOOK_URL are set in Netlify environment variables.`;
          }
          
          resolve({
            statusCode: 200,
            headers: {
              'Content-Type': 'application/json',
            },
            body: JSON.stringify({
              response_type: 'ephemeral',
              text: errorMessage,
              blocks: [{
                type: 'section',
                text: {
                  type: 'mrkdwn',
                  text: `${errorMessage}\n\n*Troubleshooting:*\n• Ensure the database file (\`oye.db\`) is included in the deployment\n• Verify TIMECAMP_API_KEY and SLACK_WEBHOOK_URL are set in Netlify environment variables\n• Check the Netlify build logs for any deployment issues\n• Contact administrator if this persists`
                }
              }]
            }),
          });
        }
      });

      child.on('error', async (error) => {
        clearTimeout(timeout); // Clear the timeout since we got an error
        
        console.error(`Failed to start command ${command}:`, error);
        console.error(`Error code: ${error.code}`);
        console.error(`Error errno: ${error.errno}`);
        console.error(`Error syscall: ${error.syscall}`);
        console.error(`Error path: ${error.path}`);
        console.error(`Attempted to execute: ${goExecutable}`);
        console.error(`Tried paths:`, possiblePaths);
        
        // Check if binary exists at each possible path
        console.error('Path existence check:');
        for (const possiblePath of possiblePaths) {
          try {
            const stats = fs.statSync(possiblePath);
            console.error(`  ${possiblePath}: EXISTS, size: ${stats.size}, executable: ${(stats.mode & parseInt('111', 8)) !== 0}`);
          } catch (fsError) {
            console.error(`  ${possiblePath}: NOT FOUND (${fsError.message})`);
          }
        }
        
        // Provide a helpful error message for Slack
        const errorMessage = error.code === 'ENOENT' 
          ? `❌ Error: Binary not found in Netlify environment. This indicates a deployment issue.`
          : `❌ Error: Failed to execute ${command.replace('-', ' ')} command. Error: ${error.message}`;
        
        resolve({
          statusCode: 200,
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({
            response_type: 'ephemeral',
            text: errorMessage,
            blocks: [{
              type: 'section',
              text: {
                type: 'mrkdwn',
                text: `${errorMessage}\n\n*Troubleshooting:*\n• Check that the binary was included in the Netlify deployment\n• Verify environment variables are set in Netlify dashboard\n• Contact administrator if this persists`
              }
            }]
          }),
        });
      });

    } catch (error) {
      console.error('Error in processSlackCommand:', error);
      
      resolve({
        statusCode: 500,
        body: JSON.stringify({ error: 'Internal server error' }),
      });
    }
  });
}

// Send a delayed response to Slack using the response_url
async function sendDelayedResponse(responseUrl, message) {
  return new Promise((resolve, reject) => {
    try {
      const url = new URL(responseUrl);
      const postData = JSON.stringify(message);
      
      const options = {
        hostname: url.hostname,
        port: url.port || (url.protocol === 'https:' ? 443 : 80),
        path: url.pathname + url.search,
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Content-Length': Buffer.byteLength(postData)
        }
      };

      const request = (url.protocol === 'https:' ? https : http).request(options, (res) => {
        let responseBody = '';
        
        res.on('data', (chunk) => {
          responseBody += chunk;
        });
        
        res.on('end', () => {
          if (res.statusCode >= 200 && res.statusCode < 300) {
            console.log('Successfully sent delayed response to Slack');
            resolve();
          } else {
            console.error(`Failed to send delayed response: ${res.statusCode} ${res.statusMessage}`);
            console.error('Response body:', responseBody);
            reject(new Error(`HTTP ${res.statusCode}: ${res.statusMessage}`));
          }
        });
      });

      request.on('error', (error) => {
        console.error('Error sending delayed response to Slack:', error);
        reject(error);
      });

      request.write(postData);
      request.end();
      
    } catch (error) {
      console.error('Error in sendDelayedResponse:', error);
      reject(error);
    }
  });
}
