<?php

// Tusk Native Engine - Worker Script
// This script runs the Tusk Framework if available, or falls back to an NDJSON echo server.

$autoloadPaths = [
    __DIR__ . '/vendor/autoload.php',
    __DIR__ . '/../vendor/autoload.php', // Depending on folder structure
];

$autoloadFile = null;
foreach ($autoloadPaths as $path) {
    if (file_exists($path)) {
        $autoloadFile = $path;
        break;
    }
}

if ($autoloadFile) {
    require_once $autoloadFile;

    if (class_exists(\Tusk\Runtime\Kernel::class) && class_exists(\Tusk\Runtime\Adapters\NativeLoopAdapter::class)) {
        // Tusk framework is available, boot it!
        $container = new \Tusk\Core\Container\Container();
        $kernel = new \Tusk\Runtime\Kernel($container, new \Tusk\Runtime\Adapters\NativeLoopAdapter());
        
        $kernel->start();
        exit(0);
    }
}

// -----------------------------------------------------------------------------
// Fallback logic if framework is not installed
// -----------------------------------------------------------------------------

// Unbuffer stdout to ensure Go receives data immediately
stream_set_write_buffer(STDOUT, 0);

while (true) {
    // 1. Read Line (Blocking)
    $line = fgets(STDIN);
    if ($line === false) {
        break; // End of pipe
    }

    // 2. Parse Request
    $req = json_decode($line, true);
    if (!$req) {
        continue;
    }

    $method = $req['method'] ?? 'GET';
    $url = $req['url'] ?? '/';
    $headers = $req['headers'] ?? [];
    $body = $req['body'] ?? '';

    // Simple Echo Logic for testing
    $responseBody = json_encode([
        'message' => 'Hello from Tusk Native Engine! (Fallback mode, framework not found)',
        'received' => [
            'method' => $method,
            'url' => $url,
            'headers' => $headers,
            'body_size' => strlen($body),
        ],
        'timestamp' => time(),
    ]);

    $response = [
        'status' => 200,
        'headers' => [
            'Content-Type' => ['application/json'],
            'X-Tusk-Worker' => [(string) getmypid()],
        ],
        'body' => $responseBody,
    ];

    // 4. Send Response (Unbuffered)
    fwrite(STDOUT, json_encode($response) . "\n");
}
