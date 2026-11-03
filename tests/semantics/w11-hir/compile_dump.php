<?php
// Compiles every corpus file with `php -l` and prints one JSON line per file:
// [name, [[level, message, line], ...]]. Run in the pinned PHP 8.5.12 image:
//   php -n compile_dump.php /corpus > compile-reference.jsonl
// The messages are what PHP reports while compiling, before any code runs.
$dir = $argv[1];
$files = glob("$dir/*.php");
sort($files);
foreach ($files as $path) {
    $cmd = 'php -n -d display_errors=1 -d log_errors=0 -d error_reporting=-1 -d html_errors=0 -l '
        . escapeshellarg($path) . ' 2>&1';
    $out = shell_exec($cmd) ?? '';
    $events = [];
    $quoted = preg_quote($path, '/');
    if (preg_match_all('/^(Fatal error|Parse error|Warning|Deprecated|Notice): (.*) in ' . $quoted . ' on line (\d+)$/m', $out, $m, PREG_SET_ORDER)) {
        foreach ($m as $e) {
            $events[] = [$e[1], $e[2], (int)$e[3]];
        }
    }
    echo json_encode([basename($path), $events], JSON_INVALID_UTF8_SUBSTITUTE | JSON_UNESCAPED_SLASHES), "\n";
}
