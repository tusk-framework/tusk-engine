<?php
stream_set_write_buffer(STDOUT, 0);
while (true) {
    if (($line = fgets(STDIN)) === false)
        break;
    $req = json_decode($line, true);

    if (!is_array($req)) {
        continue;
    }

    // Populate superglobals
    $_SERVER['REQUEST_METHOD'] = $req['method'] ?? 'GET';
    $_SERVER['REQUEST_URI'] = $req['url'] ?? '/';
    
    $_GET = $req['query'] ?? [];
    $_POST = $req['parsedBody'] ?? [];
    $_FILES = $req['uploadedFiles'] ?? [];
    $_COOKIE = $req['cookies'] ?? [];
    
    foreach ($req['headers'] ?? [] as $k => $v) {
        $key = 'HTTP_' . str_replace('-', '_', strtoupper($k));
        $_SERVER[$key] = is_array($v) ? implode(',', $v) : $v;
    }

    $body = "Superglobals populated!\n";
    $body .= "GET: " . json_encode($_GET) . "\n";
    $body .= "POST: " . json_encode($_POST) . "\n";
    $body .= "FILES: " . json_encode($_FILES) . "\n";
    
    // In a real framework, you would require "index.php" or pass this to the kernel here.

    $response = [
        'status' => 200,
        'headers' => array_merge($req['headers'] ?? [], ['Content-Type' => ['text/plain']]),
        'body' => $body
    ];

    if (isset($_GET['sleep'])) {
        usleep((int)$_GET['sleep'] * 1000);
    }

    echo json_encode($response) . "\n";
}
