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

    // Send immediate response to Slack
    const immediateResponse = {
      statusCode: 200,
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        response_type: 'ephemeral',
        text: `⏳ Generating ${command.replace('-', ' ')}...`
      }),
    };

    // Process the command asynchronously by calling the Go binary
    processSlackCommand(command, responseUrl);

    return immediateResponse;

  } catch (error) {
    console.error('Error processing Slack command:', error);
    return {
      statusCode: 500,
      body: JSON.stringify({ error: 'Internal server error' }),
    };
  }
};

// Process the Slack command asynchronously
async function processSlackCommand(command, responseUrl) {
  try {
    console.log(`Processing ${command} command...`);
    
    // Execute the Go binary with the appropriate command and response URL
    const goExecutable = './observe-yor-estimates';
    const child = spawn(goExecutable, [command, `--response-url=${responseUrl}`], {
      stdio: ['pipe', 'pipe', 'pipe'],
      env: process.env
    });

    let stdout = '';
    let stderr = '';

    child.stdout.on('data', (data) => {
      stdout += data.toString();
    });

    child.stderr.on('data', (data) => {
      stderr += data.toString();
    });

    child.on('close', async (code) => {
      if (code === 0) {
        console.log(`Successfully executed ${command} command`);
        // The Go binary handled sending the response via the response URL
      } else {
        console.error(`Command ${command} failed with code ${code}`);
        console.error('stderr:', stderr);
        
        // Only send error response if the Go binary didn't handle it
        // (Go binary should handle most errors by sending responses via response URL)
        await sendDelayedResponse(responseUrl, {
          response_type: 'in_channel',
          text: `❌ Error: ${command.replace('-', ' ')} command failed`,
          blocks: [
            {
              type: 'section',
              text: {
                type: 'mrkdwn',
                text: `❌ *${command.replace('-', ' ')} failed*\n\nThe command failed to execute properly. Please check the logs or try again later.`
              }
            }
          ]
        });
      }
    });

    child.on('error', async (error) => {
      console.error(`Failed to start command ${command}:`, error);
      
      // Send error response when we can't even start the process
      await sendDelayedResponse(responseUrl, {
        response_type: 'in_channel',
        text: `❌ Error: Failed to start ${command.replace('-', ' ')} command`,
        blocks: [
          {
            type: 'section',
            text: {
              type: 'mrkdwn',
              text: `❌ *Failed to start ${command.replace('-', ' ')}*\n\nError: ${error.message}`
            }
          }
        ]
      });
    });

  } catch (error) {
    console.error('Error in processSlackCommand:', error);
    
    // Send error response to Slack
    await sendDelayedResponse(responseUrl, {
      response_type: 'in_channel',
      text: `❌ Error: Internal error processing ${command.replace('-', ' ')} command`,
      blocks: [
        {
          type: 'section',
          text: {
            type: 'mrkdwn',
            text: `❌ *Internal error*\n\nSomething went wrong while processing the ${command.replace('-', ' ')} command. Please try again later.`
          }
        }
      ]
    });
  }
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
