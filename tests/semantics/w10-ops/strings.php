<?php
// Emits PHP's conversions and comparisons for each hex-encoded string read
// from standard input, one per line. Encodings match matrix.php.
require __DIR__ . '/enc.php';
$diags = [];
set_error_handler(function ($no, $msg) use (&$diags) { $diags[] = [$no, $msg]; return true; });
$ops = [
    '(int)' => fn($s) => (int)$s, '(float)' => fn($s) => (float)$s,
    '+0' => fn($s) => $s + 0, '%7' => fn($s) => $s % 7,
    '<=>1' => fn($s) => $s <=> "1", '==1.0' => fn($s) => $s == "1.0",
    '<=>int' => fn($s) => $s <=> 10, '==0' => fn($s) => $s == 0,
];
while (($line = fgets(STDIN)) !== false) {
    $s = hex2bin(rtrim($line, "\n"));
    $row = [];
    foreach ($ops as $name => $f) {
        $diags = [];
        try {
            $r = ['r' => enc($f($s))];
        } catch (Throwable $e) {
            $r = ['e' => [get_class($e), $e->getMessage()]];
        }
        if ($diags) { $r['d'] = $diags; }
        $row[$name] = $r;
    }
    echo json_encode($row, JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR), "\n";
}
