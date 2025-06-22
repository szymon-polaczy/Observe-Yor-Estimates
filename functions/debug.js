// Netlify Function for Environment Debugging
const fs = require('fs');
const path = require('path');

exports.handler = async (event, context) => {
  try {
    const debugInfo = {
      timestamp: new Date().toISOString(),
      environment: 'netlify-debug',
      context: {
        functionName: context.functionName,
        functionVersion: context.functionVersion,
        memoryLimitInMB: context.memoryLimitInMB,
        remainingTimeInMillis: context.getRemainingTimeInMillis(),
      },
      process: {
        cwd: process.cwd(),
        platform: process.platform,
        arch: process.arch,
        nodeVersion: process.version,
      },
      env: {
        NETLIFY: process.env.NETLIFY,
        LAMBDA_TASK_ROOT: process.env.LAMBDA_TASK_ROOT,
        NETLIFY_BUILD_BASE: process.env.NETLIFY_BUILD_BASE,
        PWD: process.env.PWD,
      },
      paths: {
        __dirname: __dirname,
        __filename: __filename,
      }
    };

    // Check for binary in various locations
    const possiblePaths = [
      path.resolve(__dirname, '..', 'observe-yor-estimates'),
      path.resolve(process.cwd(), 'observe-yor-estimates'),
      './observe-yor-estimates',
      'observe-yor-estimates',
    ];

    const binaryInfo = {};
    for (const possiblePath of possiblePaths) {
      try {
        if (fs.existsSync(possiblePath)) {
          const stats = fs.statSync(possiblePath);
          binaryInfo[possiblePath] = {
            exists: true,
            size: stats.size,
            executable: (stats.mode & parseInt('111', 8)) !== 0,
            mode: stats.mode.toString(8),
          };
        } else {
          binaryInfo[possiblePath] = { exists: false };
        }
      } catch (error) {
        binaryInfo[possiblePath] = { error: error.message };
      }
    }

    // List files in current directory and parent
    const files = {};
    try {
      files.currentDir = fs.readdirSync(process.cwd()).slice(0, 20); // Limit to first 20 files
      files.parentDir = fs.readdirSync(path.join(__dirname, '..')).slice(0, 20);
      files.functionsDir = fs.readdirSync(__dirname);
    } catch (error) {
      files.error = error.message;
    }

    return {
      statusCode: 200,
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        debug: debugInfo,
        binary: binaryInfo,
        files: files,
      }, null, 2),
    };
  } catch (error) {
    return {
      statusCode: 500,
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        error: error.message,
        stack: error.stack,
      }),
    };
  }
};
