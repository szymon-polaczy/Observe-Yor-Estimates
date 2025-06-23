const { spawn } = require('child_process');
const { promisify } = require('util');

exports.handler = async (event, context) => {
  // Set timeout to prevent long-running functions
  const timeout = context.getRemainingTimeInMillis() - 1000; // Leave 1s buffer

  try {
    // Parse the request
    const body = event.body;
    const params = new URLSearchParams(body);
    
    const slackData = {
      token: params.get('token'),
      team_id: params.get('team_id'),
      channel_id: params.get('channel_id'),
      user_name: params.get('user_name'),
      command: params.get('command'),
      text: params.get('text'),
      response_url: params.get('response_url')
    };

    // Validate Slack token
    const expectedToken = process.env.SLACK_VERIFICATION_TOKEN;
    if (expectedToken && slackData.token !== expectedToken) {
      return {
        statusCode: 401,
        body: JSON.stringify({ error: 'Invalid verification token' })
      };
    }

    // Determine the period from command text
    const period = slackData.text?.trim() || 'daily';
    if (!['daily', 'weekly', 'monthly'].includes(period)) {
      return {
        statusCode: 200,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          response_type: 'ephemeral',
          text: `❌ Invalid period "${period}". Use: daily, weekly, or monthly`
        })
      };
    }

    // Send immediate response
    const immediateResponse = {
      response_type: 'ephemeral',
      text: `⏳ Preparing your ${period} update... I'll send the results shortly!`
    };

    // Start background job for actual processing
    processUpdateInBackground(period, slackData.response_url, slackData.user_name);

    return {
      statusCode: 200,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(immediateResponse)
    };

  } catch (error) {
    console.error('Error in slack-update function:', error);
    return {
      statusCode: 500,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        response_type: 'ephemeral',
        text: '❌ An error occurred while processing your request.'
      })
    };
  }
};

async function processUpdateInBackground(period, responseUrl, userName) {
  try {
    console.log(`Starting ${period} update for ${userName}`);

    // Execute the Go CLI tool
    const result = await executeGoCommand('update', [period], {
      RESPONSE_URL: responseUrl,
      OUTPUT_JSON: 'true'
    });

    if (result.error) {
      await sendErrorToSlack(responseUrl, `Failed to generate ${period} update: ${result.error}`);
    } else {
      console.log(`Successfully completed ${period} update for ${userName}`);
    }

  } catch (error) {
    console.error('Error in background processing:', error);
    await sendErrorToSlack(responseUrl, `Background processing failed: ${error.message}`);
  }
}

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

async function sendErrorToSlack(responseUrl, errorMessage) {
  if (!responseUrl) return;

  try {
    const fetch = (await import('node-fetch')).default;
    
    const response = await fetch(responseUrl, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        response_type: 'ephemeral',
        text: `❌ ${errorMessage}`
      })
    });

    if (!response.ok) {
      console.error('Failed to send error to Slack:', response.status);
    }
  } catch (error) {
    console.error('Error sending error message to Slack:', error);
  }
} 