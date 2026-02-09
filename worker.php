<?php

// Tusk Native Engine - Worker Script
// This script runs in a loop, reading requests from STDIN and writing responses to STDOUT.
// Protocol: NDJSON (Newline Delimited JSON)

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

    // 3. Process Request (Placeholder for framework boot)
    // In a real app, this would be: $response = $kernel->handle($request);

    $method = $req['method'] ?? 'GET';
    $url = $req['url'] ?? '/';
    $headers = $req['headers'] ?? [];
    $body = $req['body'] ?? '';

    // Simple Echo Logic for testing
    $responseBody = json_encode([
        'message' => 'Hello from Tusk Native Engine!',
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
            'Content-Type' => 'application/json',
            'X-Tusk-Worker' => getmypid(),
        ],
        'body' => $responseBody,
    ];

    // 4. Send Response (Unbuffered)
    fwrite(STDOUT, json_encode($response) . "\n");
}
