<?php
stream_set_write_buffer(STDOUT, 0);
while (true) {
    if (($line = fgets(STDIN)) === false)
        break;
    $req = json_decode($line, true);

    $response = [
        'status' => 200,
        'headers' => $req['headers'] ?? [],
        'body' => 'ok'
    ];

    if (isset($req['sleep'])) {
        usleep($req['sleep'] * 1000);
    }

    echo json_encode($response) . "\n";
}
