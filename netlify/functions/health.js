exports.handler = async (event, context) => {
  try {
    const health = {
      status: 'healthy',
      timestamp: new Date().toISOString(),
      version: '2.0.0',
      architecture: 'netlify-functions-go-cli',
      environment: process.env.NODE_ENV || 'production'
    };

    return {
      statusCode: 200,
      headers: { 
        'Content-Type': 'application/json',
        'Cache-Control': 'no-cache'
      },
      body: JSON.stringify(health)
    };

  } catch (error) {
    console.error('Health check failed:', error);
    return {
      statusCode: 500,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        status: 'unhealthy',
        error: error.message,
        timestamp: new Date().toISOString()
      })
    };
  }
}; 