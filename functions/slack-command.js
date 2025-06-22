// Netlify Function for Slack Commands
// This demonstrates how to integrate the Go binary with Netlify Functions

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

    // Return immediate response for Slack
    // In a real implementation, you'd want to:
    // 1. Verify the Slack request signature
    // 2. Queue the command for background processing
    // 3. Use Slack's response_url for delayed responses
    
    return {
      statusCode: 200,
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        response_type: 'ephemeral',
        text: `⏳ Processing ${command} command...`
      }),
    };

  } catch (error) {
    console.error('Error processing Slack command:', error);
    return {
      statusCode: 500,
      body: JSON.stringify({ error: 'Internal server error' }),
    };
  }
};
