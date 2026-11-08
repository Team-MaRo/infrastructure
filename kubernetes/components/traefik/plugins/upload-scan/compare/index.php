<?php
// Reports what PHP stored of a request: every upload with its size, SHA-256 and whether it holds
// the EICAR string, and whether the raw body does.
$eicar = 'X5O!P%@AP[4\PZX54(P^)7CC)7}$' . 'EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*';
$files = [];
foreach ($_FILES as $f) {
    $names = (array) $f['tmp_name'];
    $errors = (array) $f['error'];
    foreach ($names as $i => $tmp) {
        $content = ($errors[$i] === UPLOAD_ERR_OK && is_uploaded_file($tmp)) ? file_get_contents($tmp) : '';
        $files[] = [
            'error' => $errors[$i],
            'size' => strlen($content),
            'sha' => hash('sha256', $content),
            'eicar' => str_contains($content, $eicar),
        ];
    }
}
$raw = file_get_contents('php://input');
$out = json_encode(['files' => $files, 'raw_eicar' => str_contains($raw, $eicar)]);
header('Content-Type: application/json');
header('Content-Length: ' . strlen($out));
header('Connection: close');
echo $out;
