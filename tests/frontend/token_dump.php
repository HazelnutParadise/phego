<?php
// Emit one JSON line per file: [name-or-char, text(base64), line].
foreach (glob('/corpus/*.php') as $f) {
    $src = file_get_contents($f);
    $out = [];
    foreach (token_get_all($src) as $t) {
        $out[] = is_array($t) ? [token_name($t[0]), base64_encode($t[1]), $t[2]] : [$t, base64_encode($t), 0];
    }
    echo json_encode([basename($f), $out]), "\n";
}
