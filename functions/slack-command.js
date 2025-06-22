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
    const validCommands = ['daily-update', 'weekly-update', 'monthly-update'];
    
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
      const goExecutable = path.join(__dirname, '..', 'observe-yor-estimates');
      
      console.log(`Attempting to spawn: ${goExecutable} with args: [${command}, --output-json]`);
      console.log(`Current working directory: ${process.cwd()}`);
      console.log(`__dirname: ${__dirname}`);
      
      const child = spawn(goExecutable, [command, '--output-json'], {
        stdio: ['pipe', 'pipe', 'pipe'],
        env: process.env,
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
          
          resolve({
            statusCode: 200,
            headers: {
              'Content-Type': 'application/json',
            },
            body: JSON.stringify({
              response_type: 'ephemeral',
              text: `❌ Error: ${command.replace('-', ' ')} command failed`
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
        
        // Check if binary exists
        const fs = require('fs');
        try {
          const stats = fs.statSync(goExecutable);
          console.error(`Binary exists, size: ${stats.size}, executable: ${(stats.mode & parseInt('111', 8)) !== 0}`);
        } catch (fsError) {
          console.error(`Binary does not exist or cannot be accessed: ${fsError.message}`);
        }
        
        resolve({
          statusCode: 200,
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({
            response_type: 'ephemeral',
            text: `❌ Error: Failed to start ${command.replace('-', ' ')} command. Error: ${error.message}`
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
